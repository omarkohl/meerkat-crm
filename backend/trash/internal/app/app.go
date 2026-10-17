// Package app holds the use cases of the trash module. It depends on ports (the interfaces
// below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"errors"
	"time"

	"meerkat/trash/internal/domain"
)

// Trash reads and changes the actor's deleted contacts.
type Trash interface {
	List(ctx context.Context, actor domain.UserID) ([]domain.TrashedContact, error)
	// Restore brings the contact back with what was deleted with it. Event is what the
	// contact.restored webhook carries.
	Restore(ctx context.Context, actor domain.UserID, id domain.ContactID) (Restored, error)
	// Purge deletes trashed contacts for good (all of the actor's when ids is empty), and
	// their files, and returns how many.
	Purge(ctx context.Context, actor domain.UserID, ids []domain.ContactID, now time.Time) (int, error)
}

type Restored struct {
	ID    domain.ContactID
	Event any
}

type Events interface {
	ContactRestored(ctx context.Context, actor domain.UserID, event any, now time.Time)
}

type Stage string

const (
	StageList    Stage = "list"
	StageRestore Stage = "restore"
	StagePurge   Stage = "purge"
	StageEmpty   Stage = "empty"
)

// Failure wraps an infrastructure error with the stage it happened in. A contact that is not
// in the trash is not a Failure: it is domain.ErrNotInTrash.
type Failure struct {
	Stage Stage
	Err   error
}

func (f *Failure) Error() string { return string(f.Stage) + ": " + f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

type UseCases struct {
	trash         Trash
	events        Events
	retentionDays int
}

func New(trash Trash, events Events, configuredRetentionDays int) *UseCases {
	return &UseCases{trash: trash, events: events, retentionDays: domain.Retention(configuredRetentionDays)}
}

type Listing struct {
	Contacts      []domain.TrashedContact
	RetentionDays int
}

func (u *UseCases) ListTrash(ctx context.Context, actor domain.UserID) (Listing, error) {
	list, err := u.trash.List(ctx, actor)
	if err != nil {
		return Listing{}, &Failure{Stage: StageList, Err: err}
	}
	return Listing{Contacts: list, RetentionDays: u.retentionDays}, nil
}

// PurgeAt is when a listed contact is removed by the automatic purge.
func (u *UseCases) PurgeAt(c domain.TrashedContact) time.Time {
	return domain.PurgeAt(c.DeletedAt, u.retentionDays)
}

func (u *UseCases) RestoreContact(ctx context.Context, actor domain.UserID, id domain.ContactID, now time.Time) (domain.ContactID, error) {
	r, err := u.trash.Restore(ctx, actor, id)
	if errors.Is(err, domain.ErrNotInTrash) {
		return 0, err
	}
	if err != nil {
		return 0, &Failure{Stage: StageRestore, Err: err}
	}
	u.events.ContactRestored(ctx, actor, r.Event, now)
	return r.ID, nil
}

func (u *UseCases) PurgeContact(ctx context.Context, actor domain.UserID, id domain.ContactID, now time.Time) error {
	n, err := u.trash.Purge(ctx, actor, []domain.ContactID{id}, now)
	if err != nil {
		return &Failure{Stage: StagePurge, Err: err}
	}
	if n == 0 {
		return domain.ErrNotInTrash
	}
	return nil
}

func (u *UseCases) EmptyTrash(ctx context.Context, actor domain.UserID, now time.Time) (int, error) {
	n, err := u.trash.Purge(ctx, actor, nil, now)
	if err != nil {
		return 0, &Failure{Stage: StageEmpty, Err: err}
	}
	return n, nil
}
