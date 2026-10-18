// Package app holds the use cases of the calendars module. It depends on ports (the interfaces
// below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"errors"

	"meerkat/calendars/internal/domain"
	"meerkat/clock"
	"meerkat/httputil"
)

// Subscriptions stores the actor's calendar subscriptions. Find, Update, Delete and Sync
// return domain.ErrNotFound for a subscription that is not the actor's.
type Subscriptions interface {
	List(ctx context.Context, actor domain.UserID) ([]domain.Subscription, error)
	Count(ctx context.Context, actor domain.UserID) (int, error)
	Create(ctx context.Context, actor domain.UserID, s domain.Subscription, encryptedPassword string) (domain.Subscription, error)
	Find(ctx context.Context, actor domain.UserID, id domain.SubscriptionID) (domain.Subscription, error)
	Update(ctx context.Context, actor domain.UserID, s domain.Subscription, password domain.PasswordChange, encryptedPassword string) (domain.Subscription, error)
	Delete(ctx context.Context, actor domain.UserID, id domain.SubscriptionID) error
	Sync(ctx context.Context, actor domain.UserID, id domain.SubscriptionID, clk clock.Clock) (domain.SyncStats, error)
}

type Secrets interface {
	Encrypt(plain string) (string, error)
}

type Stage string

const (
	StageList   Stage = "list"
	StageCount  Stage = "count"
	StageCreate Stage = "create"
	StageFind   Stage = "find"
	StageUpdate Stage = "update"
	StageDelete Stage = "delete"
	StageSecret Stage = "secret"
)

type Failure struct {
	Stage Stage
	Err   error
}

func (f *Failure) Error() string { return string(f.Stage) + ": " + f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

type UseCases struct {
	subscriptions Subscriptions
	secrets       Secrets
}

func New(subscriptions Subscriptions, secrets Secrets) *UseCases {
	return &UseCases{subscriptions: subscriptions, secrets: secrets}
}

func fail(stage Stage, err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return &Failure{Stage: stage, Err: err}
}

func (u *UseCases) List(ctx context.Context, actor domain.UserID) ([]domain.Subscription, error) {
	list, err := u.subscriptions.List(ctx, actor)
	if err != nil {
		return nil, fail(StageList, err)
	}
	return list, nil
}

func (u *UseCases) Create(ctx context.Context, actor domain.UserID, in domain.Input) (domain.Subscription, error) {
	n, err := u.subscriptions.Count(ctx, actor)
	if err != nil {
		return domain.Subscription{}, fail(StageCount, err)
	}
	if n >= domain.MaxPerUser {
		return domain.Subscription{}, domain.ErrLimit
	}
	s, err := domain.New(in)
	if err != nil {
		return domain.Subscription{}, err
	}
	encrypted, err := u.secrets.Encrypt(in.Password)
	if err != nil {
		return domain.Subscription{}, fail(StageSecret, err)
	}
	created, err := u.subscriptions.Create(ctx, actor, s, encrypted)
	if err != nil {
		return domain.Subscription{}, fail(StageCreate, err)
	}
	return created, nil
}

func (u *UseCases) Update(ctx context.Context, actor domain.UserID, id domain.SubscriptionID, in domain.Input) (domain.Subscription, error) {
	existing, err := u.subscriptions.Find(ctx, actor, id)
	if err != nil {
		return domain.Subscription{}, fail(StageFind, err)
	}
	changed, password, err := existing.Change(in, httputil.SameServer)
	if err != nil {
		return domain.Subscription{}, err
	}
	var encrypted string
	if password.Mode == domain.SetPassword {
		if encrypted, err = u.secrets.Encrypt(password.Plain); err != nil {
			return domain.Subscription{}, fail(StageSecret, err)
		}
	}
	saved, err := u.subscriptions.Update(ctx, actor, changed, password, encrypted)
	if err != nil {
		return domain.Subscription{}, fail(StageUpdate, err)
	}
	return saved, nil
}

func (u *UseCases) Delete(ctx context.Context, actor domain.UserID, id domain.SubscriptionID) error {
	if err := u.subscriptions.Delete(ctx, actor, id); err != nil {
		return fail(StageDelete, err)
	}
	return nil
}

func (u *UseCases) Sync(ctx context.Context, actor domain.UserID, id domain.SubscriptionID, clk clock.Clock) (domain.SyncStats, error) {
	stats, err := u.subscriptions.Sync(ctx, actor, id, clk)
	var syncErr *domain.SyncError
	if err != nil && !errors.As(err, &syncErr) {
		return domain.SyncStats{}, fail(StageFind, err)
	}
	return stats, err
}
