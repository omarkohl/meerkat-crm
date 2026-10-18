// Package app holds the use cases of the monicaimport module. It depends on ports (the
// interfaces below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"

	"meerkat/monicaimport/internal/domain"

	"github.com/rs/zerolog"
)

// Wizard is the import from Monica: connecting, the background fetch, the in-memory sessions
// that hold the API token, and applying the chosen actions. Its errors may carry their own
// HTTP status; the HTTP adapter passes those on.
type Wizard interface {
	Connect(ctx context.Context, actor domain.UserID, baseURL, apiToken string) (domain.Connection, error)
	StartFetch(ctx context.Context, actor domain.UserID, opts domain.FetchOptions, log *zerolog.Logger) error
	Status(actor domain.UserID, session string) (domain.Status, error)
	Preview(actor domain.UserID, session string) (domain.Preview, error)
	Confirm(ctx context.Context, actor domain.UserID, session string, actions []domain.Action, log *zerolog.Logger) (domain.Result, error)
	Cancel(actor domain.UserID, session string) error
}

type UseCases struct{ wizard Wizard }

func New(wizard Wizard) *UseCases { return &UseCases{wizard: wizard} }

func (u *UseCases) Connect(ctx context.Context, actor domain.UserID, baseURL, apiToken string) (domain.Connection, error) {
	return u.wizard.Connect(ctx, actor, baseURL, apiToken)
}

func (u *UseCases) StartFetch(ctx context.Context, actor domain.UserID, opts domain.FetchOptions, log *zerolog.Logger) error {
	return u.wizard.StartFetch(ctx, actor, opts, log)
}

func (u *UseCases) Status(actor domain.UserID, session string) (domain.Status, error) {
	if session == "" {
		return domain.Status{}, domain.ErrNoSession
	}
	return u.wizard.Status(actor, session)
}

func (u *UseCases) Preview(actor domain.UserID, session string) (domain.Preview, error) {
	if session == "" {
		return domain.Preview{}, domain.ErrNoSession
	}
	return u.wizard.Preview(actor, session)
}

func (u *UseCases) Confirm(ctx context.Context, actor domain.UserID, session string, actions []domain.Action, log *zerolog.Logger) (domain.Result, error) {
	return u.wizard.Confirm(ctx, actor, session, actions, log)
}

func (u *UseCases) Cancel(actor domain.UserID, session string) error {
	if session == "" {
		return domain.ErrNoSession
	}
	return u.wizard.Cancel(actor, session)
}
