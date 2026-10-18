// Package app holds the use cases of the tokens module. It depends on ports (the interfaces
// below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"errors"
	"time"

	"meerkat/tokens/internal/domain"
)

// Tokens stores the actor's API tokens.
type Tokens interface {
	// List returns every token including revoked ones, newest first.
	List(ctx context.Context, actor domain.UserID) ([]domain.APIToken, error)
	Create(ctx context.Context, actor domain.UserID, name, hash string) (domain.APIToken, error)
	// Revoke returns domain.ErrTokenNotFound for a token that is not the actor's.
	Revoke(ctx context.Context, actor domain.UserID, id domain.TokenID, now time.Time) (domain.APIToken, error)
}

// Feeds stores the calendar feed: at most one per user.
type Feeds interface {
	// Find returns nil when the actor has no feed.
	Find(ctx context.Context, actor domain.UserID) (*domain.Feed, error)
	// Replace gives the actor a feed with this token hash, dropping any earlier one.
	Replace(ctx context.Context, actor domain.UserID, hash string) (domain.Feed, error)
	Delete(ctx context.Context, actor domain.UserID) error
	// ByHash returns domain.ErrFeedNotFound when no feed has the hash or its owner is gone.
	ByHash(ctx context.Context, hash string) (domain.Feed, error)
	Render(ctx context.Context, owner domain.UserID, language string) (string, error)
	// RecordFetch never fails the fetch: the port logs what goes wrong.
	RecordFetch(ctx context.Context, id domain.FeedID, at time.Time)
}

type Audit interface {
	Record(ctx context.Context, actor domain.UserID, event, detail string, client domain.Client)
}

type Stage string

const (
	StageGenerate    Stage = "generate"
	StageList        Stage = "list"
	StageCreate      Stage = "create"
	StageRevoke      Stage = "revoke"
	StageFeedStatus  Stage = "feed-status"
	StageFeedReplace Stage = "feed-replace"
	StageFeedDelete  Stage = "feed-delete"
	StageFeedLookup  Stage = "feed-lookup"
	StageFeedBuild   Stage = "feed-build"
)

// Failure wraps an infrastructure error with the stage it happened in.
type Failure struct {
	Stage Stage
	Err   error
}

func (f *Failure) Error() string { return string(f.Stage) + ": " + f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

type UseCases struct {
	tokens Tokens
	feeds  Feeds
	audit  Audit
}

func New(tokens Tokens, feeds Feeds, audit Audit) *UseCases {
	return &UseCases{tokens: tokens, feeds: feeds, audit: audit}
}

func (u *UseCases) ListTokens(ctx context.Context, actor domain.UserID) ([]domain.APIToken, error) {
	list, err := u.tokens.List(ctx, actor)
	if err != nil {
		return nil, &Failure{Stage: StageList, Err: err}
	}
	return list, nil
}

// CreateToken returns the token and its plaintext, which exists only in this result.
func (u *UseCases) CreateToken(ctx context.Context, actor domain.UserID, name string, client domain.Client) (domain.APIToken, string, error) {
	plaintext, hash, err := domain.NewAPITokenSecret()
	if err != nil {
		return domain.APIToken{}, "", &Failure{Stage: StageGenerate, Err: err}
	}
	token, err := u.tokens.Create(ctx, actor, name, hash)
	if err != nil {
		return domain.APIToken{}, "", &Failure{Stage: StageCreate, Err: err}
	}
	u.audit.Record(ctx, actor, domain.EventAPITokenCreated, domain.TokenAuditDetail(token.Name), client)
	return token, plaintext, nil
}

func (u *UseCases) RevokeToken(ctx context.Context, actor domain.UserID, id domain.TokenID, client domain.Client, now time.Time) error {
	token, err := u.tokens.Revoke(ctx, actor, id, now)
	if errors.Is(err, domain.ErrTokenNotFound) {
		return err
	}
	if err != nil {
		return &Failure{Stage: StageRevoke, Err: err}
	}
	u.audit.Record(ctx, actor, domain.EventAPITokenRevoked, domain.TokenAuditDetail(token.Name), client)
	return nil
}

// FeedStatus returns nil when the actor has no feed.
func (u *UseCases) FeedStatus(ctx context.Context, actor domain.UserID) (*domain.Feed, error) {
	f, err := u.feeds.Find(ctx, actor)
	if err != nil {
		return nil, &Failure{Stage: StageFeedStatus, Err: err}
	}
	return f, nil
}

// CreateFeed makes or replaces the actor's feed; the old URL stops working. The token is
// returned only here.
func (u *UseCases) CreateFeed(ctx context.Context, actor domain.UserID, client domain.Client) (domain.Feed, string, error) {
	token, hash, err := domain.NewFeedToken()
	if err != nil {
		return domain.Feed{}, "", &Failure{Stage: StageGenerate, Err: err}
	}
	f, err := u.feeds.Replace(ctx, actor, hash)
	if err != nil {
		return domain.Feed{}, "", &Failure{Stage: StageFeedReplace, Err: err}
	}
	u.audit.Record(ctx, actor, domain.EventFeedCreated, "", client)
	return f, token, nil
}

func (u *UseCases) RevokeFeed(ctx context.Context, actor domain.UserID, client domain.Client) error {
	if err := u.feeds.Delete(ctx, actor); err != nil {
		return &Failure{Stage: StageFeedDelete, Err: err}
	}
	u.audit.Record(ctx, actor, domain.EventFeedRevoked, "", client)
	return nil
}

// ServeFeed builds the calendar of whoever owns the feed behind file (<token>.ics). The token
// is the only credential, so every way it can fail to match is domain.ErrFeedNotFound.
func (u *UseCases) ServeFeed(ctx context.Context, file string, now time.Time) (string, error) {
	token, ok := domain.ParseFeedFile(file)
	if !ok {
		return "", domain.ErrFeedNotFound
	}
	hash := domain.HashSecret(token)
	feed, err := u.feeds.ByHash(ctx, hash)
	if errors.Is(err, domain.ErrFeedNotFound) {
		return "", err
	}
	if err != nil {
		return "", &Failure{Stage: StageFeedLookup, Err: err}
	}
	if !domain.SameHash(feed.TokenHash, hash) {
		return "", domain.ErrFeedNotFound
	}
	body, err := u.feeds.Render(ctx, feed.Owner, domain.FeedLanguage(feed.Language))
	if err != nil {
		return "", &Failure{Stage: StageFeedBuild, Err: err}
	}
	u.feeds.RecordFetch(ctx, feed.ID, now)
	return body, nil
}
