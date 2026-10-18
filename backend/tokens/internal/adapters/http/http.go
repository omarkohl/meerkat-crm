// Package httpadapter exposes the tokens use cases as huma operations. It maps HTTP to the
// use cases and their results and errors back to the responses the API always sent
// (docs/adr/0012-module-layout.md). The calendar feed itself is a plain Gin handler: it sends
// a calendar, not JSON.
package httpadapter

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"meerkat/api"
	apperrors "meerkat/errors"
	"meerkat/idutil"
	"meerkat/middleware"
	"meerkat/tokens/internal/app"
	"meerkat/tokens/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
)

// Register adds the management operations to a on protected, the authenticated group, and
// the feed on public. Calendar apps cannot send cookies or headers, so the token in the path
// is the feed's only credential.
func Register(a huma.API, public, protected gin.IRoutes, uc *app.UseCases) {
	h := &handlers{uc: uc}
	tokens, feed := []string{"api-tokens"}, []string{"calendar-feed"}
	api.RegisterOn(a, protected, huma.Operation{
		OperationID: "list-api-tokens", Method: http.MethodGet, Path: "/api-tokens",
		Summary: "The user's API tokens, newest first, revoked ones included", Tags: tokens,
	}, h.list)
	api.RegisterOn(a, protected, huma.Operation{
		OperationID: "create-api-token", Method: http.MethodPost, Path: "/api-tokens", DefaultStatus: http.StatusCreated,
		Summary: "Create an API token; the plaintext is in this response only", Tags: tokens,
	}, h.create)
	api.RegisterOn(a, protected, huma.Operation{
		OperationID: "revoke-api-token", Method: http.MethodDelete, Path: "/api-tokens/{id}",
		Summary: "Revoke an API token", Tags: tokens,
	}, h.revoke)
	api.RegisterOn(a, protected, huma.Operation{
		OperationID: "get-calendar-feed", Method: http.MethodGet, Path: "/ics-feed",
		Summary: "Whether the user has a calendar feed; never the token", Tags: feed,
	}, h.feedStatus)
	api.RegisterOn(a, protected, huma.Operation{
		OperationID: "create-calendar-feed", Method: http.MethodPost, Path: "/ics-feed", DefaultStatus: http.StatusCreated,
		Summary: "Create the calendar feed or replace its token; the old URL stops working", Tags: feed,
	}, h.feedCreate)
	api.RegisterOn(a, protected, huma.Operation{
		OperationID: "revoke-calendar-feed", Method: http.MethodDelete, Path: "/ics-feed",
		Summary: "Revoke the calendar feed (idempotent)", Tags: feed,
	}, h.feedRevoke)
	if public == nil { // cmd/openapi describes operations only; the feed is not one
		return
	}
	public.GET("/ics/:file", middleware.APIRateLimitMiddleware(), h.serveFeed)
}

type handlers struct{ uc *app.UseCases }

type APITokenResponse struct {
	ID         uint       `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at" nullable:"true"`
	RevokedAt  *time.Time `json:"revoked_at" nullable:"true"`
}

type APITokenCreatedResponse struct {
	APITokenResponse
	Token string `json:"token" doc:"The plaintext token; it cannot be read again"`
}

type APITokenBody struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}

type CreateAPITokenInput struct{ Body APITokenBody }

func (i *CreateAPITokenInput) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	return api.ValidateBody(prefix, &i.Body)
}

// TokenPath takes the ID as text so that one that is not a number is a 400 of the API's
// own wording.
type TokenPath struct {
	ID string `path:"id" doc:"The token's ID"`
}

type APITokenListOutput struct {
	Body struct {
		Tokens []APITokenResponse `json:"tokens"`
	}
}

type APITokenCreatedOutput struct{ Body APITokenCreatedResponse }

type MessageOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

type FeedStatusResponse struct {
	Enabled       bool       `json:"enabled"`
	CreatedAt     *time.Time `json:"created_at" nullable:"true"`
	LastFetchedAt *time.Time `json:"last_fetched_at" nullable:"true" doc:"When the feed URL was last read; recorded at most once a minute"`
}

type FeedCreatedResponse struct {
	FeedStatusResponse
	Token string `json:"token" doc:"The secret in the feed URL; it cannot be read again"`
}

type FeedStatusOutput struct{ Body FeedStatusResponse }

type FeedCreatedOutput struct{ Body FeedCreatedResponse }

func actorOf(ctx context.Context) (domain.UserID, error) {
	id, ok := api.UserID(ctx)
	if !ok {
		return 0, apperrors.ErrUnauthorized("Authentication required")
	}
	return domain.UserID(id), nil
}

func client(ctx context.Context) domain.Client {
	return domain.Client{IP: api.ClientIP(ctx), UserAgent: api.UserAgent(ctx)}
}

func tokenResponse(t domain.APIToken) APITokenResponse {
	return APITokenResponse{ID: uint(t.ID), Name: t.Name, CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, RevokedAt: t.RevokedAt}
}

func (h *handlers) list(ctx context.Context, _ *struct{}) (*APITokenListOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	tokens, err := h.uc.ListTokens(ctx, actor)
	if err != nil {
		return nil, toAppError(err)
	}
	out := &APITokenListOutput{}
	out.Body.Tokens = make([]APITokenResponse, len(tokens))
	for i, t := range tokens {
		out.Body.Tokens[i] = tokenResponse(t)
	}
	return out, nil
}

func (h *handlers) create(ctx context.Context, in *CreateAPITokenInput) (*APITokenCreatedOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	token, plaintext, err := h.uc.CreateToken(ctx, actor, in.Body.Name, client(ctx))
	if err != nil {
		return nil, toAppError(err)
	}
	return &APITokenCreatedOutput{Body: APITokenCreatedResponse{APITokenResponse: tokenResponse(token), Token: plaintext}}, nil
}

func (h *handlers) revoke(ctx context.Context, in *TokenPath) (*MessageOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	id, err := idutil.Parse(in.ID)
	if err != nil {
		return nil, apperrors.ErrInvalidInput("id", "must be a positive integer")
	}
	if err := h.uc.RevokeToken(ctx, actor, domain.TokenID(id), client(ctx), api.Now(ctx)); err != nil {
		return nil, toAppError(err)
	}
	out := &MessageOutput{}
	out.Body.Message = "Token revoked successfully"
	return out, nil
}

func feedStatus(f *domain.Feed) FeedStatusResponse {
	if f == nil {
		return FeedStatusResponse{}
	}
	created := f.CreatedAt
	return FeedStatusResponse{Enabled: true, CreatedAt: &created, LastFetchedAt: f.LastFetchedAt}
}

func (h *handlers) feedStatus(ctx context.Context, _ *struct{}) (*FeedStatusOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	f, err := h.uc.FeedStatus(ctx, actor)
	if err != nil {
		return nil, toAppError(err)
	}
	return &FeedStatusOutput{Body: feedStatus(f)}, nil
}

func (h *handlers) feedCreate(ctx context.Context, _ *struct{}) (*FeedCreatedOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	f, token, err := h.uc.CreateFeed(ctx, actor, client(ctx))
	if err != nil {
		return nil, toAppError(err)
	}
	return &FeedCreatedOutput{Body: FeedCreatedResponse{FeedStatusResponse: feedStatus(&f), Token: token}}, nil
}

func (h *handlers) feedRevoke(ctx context.Context, _ *struct{}) (*MessageOutput, error) {
	actor, err := actorOf(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.uc.RevokeFeed(ctx, actor, client(ctx)); err != nil {
		return nil, toAppError(err)
	}
	out := &MessageOutput{}
	out.Body.Message = "Calendar feed revoked successfully"
	return out, nil
}

// serveFeed ignores any session: the data is always the feed owner's. Every failure to match
// is a 404 so tokens cannot be probed.
func (h *handlers) serveFeed(c *gin.Context) {
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Robots-Tag", "noindex")
	body, err := h.uc.ServeFeed(c.Request.Context(), c.Param("file"), middleware.RequestNow(c))
	if errors.Is(err, domain.ErrFeedNotFound) {
		c.Header("Cache-Control", "no-store")
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Calendar feed"))
		return
	}
	if err != nil {
		apperrors.AbortWithError(c, toAppError(err))
		return
	}
	sum := sha256.Sum256([]byte(body))
	etag := fmt.Sprintf(`"%x"`, sum[:16])
	c.Header("ETag", etag)
	c.Header("Cache-Control", "private, max-age=900")
	if match := c.GetHeader("If-None-Match"); match != "" && strings.Contains(match, etag) {
		c.Status(http.StatusNotModified)
		return
	}
	c.Header("Content-Disposition", `inline; filename="meerkat.ics"`)
	c.Data(http.StatusOK, "text/calendar; charset=utf-8", []byte(body))
}

// toAppError words a use-case error the way the Gin handlers did.
func toAppError(err error) *apperrors.AppError {
	if errors.Is(err, domain.ErrTokenNotFound) {
		return apperrors.ErrNotFound("API token")
	}
	var f *app.Failure
	if errors.As(err, &f) {
		if f.Stage == app.StageGenerate {
			return apperrors.ErrInternal("token generation failed").WithError(f.Err)
		}
		msg := map[app.Stage]string{
			app.StageList: "query", app.StageCreate: "insert", app.StageRevoke: "update",
			app.StageFeedStatus: "query", app.StageFeedReplace: "insert", app.StageFeedDelete: "delete",
			app.StageFeedLookup: "query", app.StageFeedBuild: "build calendar feed",
		}[f.Stage]
		if msg != "" {
			return apperrors.ErrDatabase(msg).WithError(f.Err)
		}
	}
	return apperrors.ErrInternal("Unexpected error").WithError(err)
}
