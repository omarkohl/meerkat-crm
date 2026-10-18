// Package app holds the use cases of the contactsync module and the ports they need
// (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"errors"

	"meerkat/clock"
	"meerkat/contactsync/internal/domain"
	"meerkat/httputil"
)

// Connections stores the actor's CardDAV connection: at most one per user.
type Connections interface {
	// Find returns domain.ErrNotFound when the actor has none.
	Find(ctx context.Context, actor domain.UserID) (domain.Connection, error)
	// Save writes the connection, drops its links when asked, and changes the password.
	Save(ctx context.Context, actor domain.UserID, s domain.Saved, encryptedPassword string) (domain.Connection, error)
	// Delete removes the connection and its links; contacts stay.
	Delete(ctx context.Context, actor domain.UserID) error
	// Credentials returns the stored server address and a function that decrypts the password.
	Credentials(ctx context.Context, actor domain.UserID) (baseURL string, password func() string, found bool)
	// StartSync begins a background sync and reports false when one is already running.
	StartSync(ctx context.Context, actor domain.UserID, clk clock.Clock) (bool, error)
}

type Directory interface {
	Discover(ctx context.Context, baseURL, username, password string) ([]domain.AddressBook, error)
}

type Secrets interface {
	Encrypt(plain string) (string, error)
}

type Stage string

const (
	StageFind   Stage = "find"
	StageSave   Stage = "save"
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
	connections Connections
	directory   Directory
	secrets     Secrets
}

func New(connections Connections, directory Directory, secrets Secrets) *UseCases {
	return &UseCases{connections: connections, directory: directory, secrets: secrets}
}

func fail(stage Stage, err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return &Failure{Stage: stage, Err: err}
}

func (u *UseCases) Get(ctx context.Context, actor domain.UserID) (domain.Connection, error) {
	c, err := u.connections.Find(ctx, actor)
	if err != nil {
		return domain.Connection{}, fail(StageFind, err)
	}
	return c, nil
}

func (u *UseCases) Save(ctx context.Context, actor domain.UserID, in domain.Input) (domain.Saved, error) {
	if _, err := domain.NormalizeBaseURL(in.BaseURL); err != nil {
		return domain.Saved{}, err
	}
	existing, err := u.connections.Find(ctx, actor)
	var current *domain.Connection
	switch {
	case err == nil:
		current = &existing
	case !errors.Is(err, domain.ErrNotFound):
		return domain.Saved{}, fail(StageFind, err)
	}
	saved, err := domain.Save(current, in, httputil.SameServer)
	if err != nil {
		return domain.Saved{}, err
	}
	var encrypted string
	if saved.Password.Mode == domain.SetPassword {
		if encrypted, err = u.secrets.Encrypt(saved.Password.Plain); err != nil {
			return domain.Saved{}, fail(StageSecret, err)
		}
	}
	saved.Connection, err = u.connections.Save(ctx, actor, saved, encrypted)
	if err != nil {
		return domain.Saved{}, fail(StageSave, err)
	}
	return saved, nil
}

func (u *UseCases) Delete(ctx context.Context, actor domain.UserID) error {
	if err := u.connections.Delete(ctx, actor); err != nil {
		return fail(StageDelete, err)
	}
	return nil
}

// Discover lists the address books of a server. With no password the stored one is used, but only
// for the server it was stored for.
func (u *UseCases) Discover(ctx context.Context, actor domain.UserID, baseURL, username, password string) ([]domain.AddressBook, error) {
	normalized, err := domain.NormalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if password == "" {
		if storedURL, stored, found := u.connections.Credentials(ctx, actor); found && httputil.SameServer(storedURL, normalized) {
			password = stored()
		}
	}
	return u.directory.Discover(ctx, normalized, username, password)
}

func (u *UseCases) StartSync(ctx context.Context, actor domain.UserID, clk clock.Clock) error {
	started, err := u.connections.StartSync(ctx, actor, clk)
	if err != nil {
		return fail(StageFind, err)
	}
	if !started {
		return domain.ErrSyncRunning
	}
	return nil
}
