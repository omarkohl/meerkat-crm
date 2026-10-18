// Package app holds the use cases of the webhooks module. It depends on ports (the interfaces
// below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"errors"

	"meerkat/clock"
	"meerkat/webhooks/internal/domain"
)

// Webhooks stores the actor's webhooks. Every method that takes an ID returns
// domain.ErrNotFound for a webhook that is not the actor's.
type Webhooks interface {
	List(ctx context.Context, actor domain.UserID) ([]domain.Webhook, error)
	Count(ctx context.Context, actor domain.UserID) (int, error)
	Create(ctx context.Context, actor domain.UserID, in domain.Input, secret string) (domain.Webhook, error)
	Find(ctx context.Context, actor domain.UserID, id domain.WebhookID) (domain.Webhook, error)
	Update(ctx context.Context, actor domain.UserID, id domain.WebhookID, in domain.Input) (domain.Webhook, error)
	Delete(ctx context.Context, actor domain.UserID, id domain.WebhookID) error
	// Deliveries returns the 50 latest, newest first.
	Deliveries(ctx context.Context, actor domain.UserID, id domain.WebhookID) ([]domain.Delivery, error)
}

// Sender delivers a test payload to the webhook right away, whatever its event subscriptions.
type Sender interface {
	SendTest(ctx context.Context, actor domain.UserID, id domain.WebhookID, clk clock.Clock) (domain.Delivery, error)
}

type Stage string

const (
	StageGenerate   Stage = "generate"
	StageList       Stage = "list"
	StageCount      Stage = "count"
	StageCreate     Stage = "create"
	StageFind       Stage = "find"
	StageUpdate     Stage = "update"
	StageDelete     Stage = "delete"
	StageDeliveries Stage = "deliveries"
)

// Failure wraps an infrastructure error with the stage it happened in.
type Failure struct {
	Stage Stage
	Err   error
}

func (f *Failure) Error() string { return string(f.Stage) + ": " + f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

type UseCases struct {
	webhooks Webhooks
	sender   Sender
}

func New(webhooks Webhooks, sender Sender) *UseCases {
	return &UseCases{webhooks: webhooks, sender: sender}
}

func (u *UseCases) fail(stage Stage, err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return &Failure{Stage: stage, Err: err}
}

func (u *UseCases) ListWebhooks(ctx context.Context, actor domain.UserID) ([]domain.Webhook, error) {
	list, err := u.webhooks.List(ctx, actor)
	if err != nil {
		return nil, u.fail(StageList, err)
	}
	return list, nil
}

// CreateWebhook returns the webhook with its secret, which exists only in this result.
func (u *UseCases) CreateWebhook(ctx context.Context, actor domain.UserID, in domain.Input) (domain.Webhook, string, error) {
	n, err := u.webhooks.Count(ctx, actor)
	if err != nil {
		return domain.Webhook{}, "", u.fail(StageCount, err)
	}
	if n >= domain.MaxPerUser {
		return domain.Webhook{}, "", domain.ErrLimit
	}
	secret, err := domain.NewSecret()
	if err != nil {
		return domain.Webhook{}, "", u.fail(StageGenerate, err)
	}
	w, err := u.webhooks.Create(ctx, actor, in, secret)
	if err != nil {
		return domain.Webhook{}, "", u.fail(StageCreate, err)
	}
	return w, secret, nil
}

func (u *UseCases) ShowWebhook(ctx context.Context, actor domain.UserID, id domain.WebhookID) (domain.Webhook, error) {
	w, err := u.webhooks.Find(ctx, actor, id)
	if err != nil {
		return domain.Webhook{}, u.fail(StageFind, err)
	}
	return w, nil
}

func (u *UseCases) UpdateWebhook(ctx context.Context, actor domain.UserID, id domain.WebhookID, in domain.Input) (domain.Webhook, error) {
	w, err := u.webhooks.Update(ctx, actor, id, in)
	if err != nil {
		return domain.Webhook{}, u.fail(StageUpdate, err)
	}
	return w, nil
}

func (u *UseCases) DeleteWebhook(ctx context.Context, actor domain.UserID, id domain.WebhookID) error {
	if err := u.webhooks.Delete(ctx, actor, id); err != nil {
		return u.fail(StageDelete, err)
	}
	return nil
}

func (u *UseCases) TestWebhook(ctx context.Context, actor domain.UserID, id domain.WebhookID, clk clock.Clock) (domain.Delivery, error) {
	d, err := u.sender.SendTest(ctx, actor, id, clk)
	if err != nil {
		return domain.Delivery{}, u.fail(StageFind, err)
	}
	return d, nil
}

func (u *UseCases) WebhookDeliveries(ctx context.Context, actor domain.UserID, id domain.WebhookID) ([]domain.Delivery, error) {
	list, err := u.webhooks.Deliveries(ctx, actor, id)
	if err != nil {
		return nil, u.fail(StageDeliveries, err)
	}
	return list, nil
}
