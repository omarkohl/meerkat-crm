// Package app holds the use cases of the activities module. It depends on ports (the interfaces
// below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"meerkat/activities/internal/domain"
)

const (
	defaultLimit = 25
	maxLimit     = 100
)

// Filter selects activities. From and To are days; To is inclusive.
type Filter struct {
	Search       string
	From         *time.Time
	To           *time.Time
	Page         int
	Limit        int
	WithContacts bool
}

// Activities reads activities with their contacts. Every method is scoped to the actor.
type Activities interface {
	Get(ctx context.Context, actor domain.UserID, id domain.ActivityID) (domain.Activity, error)
	ForContact(ctx context.Context, actor domain.UserID, contact domain.ContactID) ([]domain.Activity, error)
	List(ctx context.Context, actor domain.UserID, f Filter) ([]domain.Activity, int64, error)
}

type Tx interface {
	Get(ctx context.Context, actor domain.UserID, id domain.ActivityID) (domain.Activity, error)
	CountContacts(ctx context.Context, actor domain.UserID, ids []domain.ContactID) (int, error)
	Create(ctx context.Context, actor domain.UserID, a domain.Activity, contacts []domain.ContactID) (domain.Activity, error)
	// Update stores the edited fields; contacts nil keeps the links, otherwise they are replaced.
	Update(ctx context.Context, actor domain.UserID, a domain.Activity, contacts []domain.ContactID) (domain.Activity, error)
	Delete(ctx context.Context, actor domain.UserID, id domain.ActivityID) error
}

type Store interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

// Events tells the rest of the system what happened. It must not fail the use case.
type Events interface {
	ActivityCreated(ctx context.Context, actor domain.UserID, a domain.Activity, now time.Time)
	ActivityUpdated(ctx context.Context, actor domain.UserID, a domain.Activity, now time.Time)
	ActivityDeleted(ctx context.Context, actor domain.UserID, id domain.ActivityID, now time.Time)
}

type Stage string

const (
	StageRetrieve   Stage = "retrieve"
	StageContact    Stage = "contact"
	StageForContact Stage = "forcontact"
	StageList       Stage = "list"
	StageCreate     Stage = "create"
	StageUpdate     Stage = "update"
	StageDelete     Stage = "delete"
)

// Failure wraps an infrastructure error with the stage it happened in. A missing activity or
// contact is not a Failure: it is a domain error.
type Failure struct {
	Stage Stage
	Err   error
}

func (f *Failure) Error() string { return string(f.Stage) + ": " + f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

type UseCases struct {
	activities Activities
	store      Store
	events     Events
}

func New(activities Activities, store Store, events Events) *UseCases {
	return &UseCases{activities: activities, store: store, events: events}
}

func fail(stage Stage, err error) error {
	var f *Failure
	if errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrContactNotFound) ||
		errors.Is(err, domain.ErrContactsNotFound) || errors.As(err, &f) {
		return err
	}
	return &Failure{Stage: stage, Err: err}
}

func (u *UseCases) GetActivity(ctx context.Context, actor domain.UserID, id domain.ActivityID) (domain.Activity, error) {
	a, err := u.activities.Get(ctx, actor, id)
	if err != nil {
		return domain.Activity{}, fail(StageRetrieve, err)
	}
	return a, nil
}

func (u *UseCases) ActivitiesOfContact(ctx context.Context, actor domain.UserID, contact domain.ContactID) ([]domain.Activity, error) {
	list, err := u.activities.ForContact(ctx, actor, contact)
	if err != nil {
		return nil, fail(StageForContact, err)
	}
	return list, nil
}

// ListActivities returns a page of the actor's activities. Paging is normalised here.
func (u *UseCases) ListActivities(ctx context.Context, actor domain.UserID, f Filter) (list []domain.Activity, total int64, page, limit int, err error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 {
		f.Limit = defaultLimit
	}
	f.Limit = min(f.Limit, maxLimit)
	f.Search = strings.ToLower(strings.TrimSpace(f.Search))
	if f.To != nil {
		next := f.To.AddDate(0, 0, 1)
		f.To = &next
	}
	list, total, err = u.activities.List(ctx, actor, f)
	if err != nil {
		return nil, 0, 0, 0, fail(StageList, err)
	}
	return list, total, f.Page, f.Limit, nil
}

func requireContacts(ctx context.Context, tx Tx, actor domain.UserID, ids []domain.ContactID) error {
	if len(ids) == 0 {
		return nil
	}
	n, err := tx.CountContacts(ctx, actor, ids)
	if err != nil {
		return &Failure{Stage: StageContact, Err: err}
	}
	if n != len(ids) {
		return domain.ErrContactsNotFound
	}
	return nil
}

func (u *UseCases) CreateActivity(ctx context.Context, actor domain.UserID, a domain.Activity, contacts []domain.ContactID, now time.Time) (domain.Activity, error) {
	var created domain.Activity
	err := u.store.InTx(ctx, func(tx Tx) error {
		if err := requireContacts(ctx, tx, actor, contacts); err != nil {
			return err
		}
		var err error
		created, err = tx.Create(ctx, actor, a, contacts)
		return err
	})
	if err != nil {
		return domain.Activity{}, fail(StageCreate, err)
	}
	u.events.ActivityCreated(ctx, actor, created, now)
	return created, nil
}

// UpdateActivity edits an activity. contacts nil keeps its contacts; a list, even an empty
// one, replaces them. keepEndDate ignores edit.EndDate.
func (u *UseCases) UpdateActivity(ctx context.Context, actor domain.UserID, id domain.ActivityID, edit domain.Activity, keepEndDate bool, contacts []domain.ContactID, now time.Time) (domain.Activity, error) {
	var updated domain.Activity
	err := u.store.InTx(ctx, func(tx Tx) error {
		a, err := tx.Get(ctx, actor, id)
		if err != nil {
			return fail(StageRetrieve, err)
		}
		if err := requireContacts(ctx, tx, actor, contacts); err != nil {
			return err
		}
		if keepEndDate {
			edit.EndDate = a.EndDate
		}
		a.Edit(edit.Title, edit.Description, edit.Location, edit.Date, edit.EndDate)
		if a.EndsBeforeStart() {
			return domain.ErrEndBeforeStart
		}
		updated, err = tx.Update(ctx, actor, a, contacts)
		return err
	})
	if err != nil {
		return domain.Activity{}, fail(StageUpdate, err)
	}
	u.events.ActivityUpdated(ctx, actor, updated, now)
	return updated, nil
}

func (u *UseCases) DeleteActivity(ctx context.Context, actor domain.UserID, id domain.ActivityID, now time.Time) error {
	err := u.store.InTx(ctx, func(tx Tx) error {
		if _, err := tx.Get(ctx, actor, id); err != nil {
			return fail(StageRetrieve, err)
		}
		return tx.Delete(ctx, actor, id)
	})
	if err != nil {
		return fail(StageDelete, err)
	}
	u.events.ActivityDeleted(ctx, actor, id, now)
	return nil
}
