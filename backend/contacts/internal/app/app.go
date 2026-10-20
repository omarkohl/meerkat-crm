// Package app holds the use cases of the contacts module. It depends on ports (the
// interfaces below) and on the domain, never on GORM, Gin or huma
// (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"errors"
	"time"

	"meerkat/contacts/internal/domain"
)

// Contacts reads contacts. Every method is scoped to the actor: a contact of another
// user is reported as domain.ErrNotFound.
type Contacts interface {
	Get(ctx context.Context, actor domain.UserID, id domain.ContactID) (domain.Contact, error)
	// Show loads a contact for display. Without fields it is complete, with its
	// relationships; with fields only those are filled and unknown names are ignored.
	Show(ctx context.Context, actor domain.UserID, id domain.ContactID, fields []string) (domain.Contact, error)
	// List returns one page of the actor's contacts and the number of all matches.
	List(ctx context.Context, actor domain.UserID, q ListQuery) ([]domain.Contact, int64, error)
	// Random returns up to n of the actor's live contacts in random order.
	Random(ctx context.Context, actor domain.UserID, n int) ([]domain.Contact, error)
	// UpcomingBirthdays returns the actor's upcoming birthdays as of now.
	UpcomingBirthdays(ctx context.Context, actor domain.UserID, now time.Time) ([]domain.Birthday, error)
	// Circles returns the distinct circle names of the actor's live contacts.
	Circles(ctx context.Context, actor domain.UserID) ([]string, error)
}

const (
	DefaultLimit = 25
	MaxLimit     = 100
)

// ListQuery is a request for a page of contacts. Zero values mean the default; unknown
// sort fields and orders fall back to the default too.
type ListQuery struct {
	Search          string
	SearchAll       bool
	Circle          string
	Sort            string
	Order           string
	IncludeArchived bool
	ArchivedOnly    bool
	Page            int
	Limit           int
	// Fields limits the columns; empty means all. Includes names the related data to load:
	// notes, activities, relationships, reminders.
	Fields   []string
	Includes []string
}

// ContactPage is a page of contacts with the paging that produced it.
type ContactPage struct {
	Contacts []domain.Contact
	Total    int64
	Page     int
	Limit    int
}

// Tx is the set of writes a use case can do inside one transaction. Like Contacts it is
// scoped to the actor.
type Tx interface {
	// Get reads a contact inside the transaction, so a read-modify-write is atomic.
	Get(ctx context.Context, actor domain.UserID, id domain.ContactID) (domain.Contact, error)
	// Create stores a new contact for the actor and returns it as stored.
	Create(ctx context.Context, actor domain.UserID, c domain.Contact) (domain.Contact, error)
	// Update stores the edited fields of an existing contact and returns it as stored.
	Update(ctx context.Context, actor domain.UserID, c domain.Contact) (domain.Contact, error)
	DeleteReminders(ctx context.Context, actor domain.UserID, id domain.ContactID) error
	SetArchived(ctx context.Context, actor domain.UserID, id domain.ContactID, archived bool) error
	// MoveToTrash soft-deletes the contact together with its notes, reminders and
	// relationships; activity links and photo files stay so a restore loses nothing.
	MoveToTrash(ctx context.Context, actor domain.UserID, id domain.ContactID) error
}

// Store runs writes as one unit of work.
type Store interface {
	InTx(ctx context.Context, fn func(Tx) error) error
}

// Events tells the rest of the system what happened. It must not fail the use case.
type Events interface {
	ContactCreated(ctx context.Context, actor domain.UserID, c domain.Contact, now time.Time)
	ContactUpdated(ctx context.Context, actor domain.UserID, c domain.Contact, now time.Time)
	ContactDeleted(ctx context.Context, actor domain.UserID, id domain.ContactID, now time.Time)
}

// Stage says which step of a use case failed, so the caller can word the error.
type Stage string

const (
	StageRetrieve  Stage = "retrieve"
	StageCreate    Stage = "create"
	StageUpdate    Stage = "update"
	StageArchive   Stage = "archive"
	StageUnarchive Stage = "unarchive"
	StageDelete    Stage = "delete"
	StageCircles   Stage = "circles"
	StageList      Stage = "list"
	StageBirthdays Stage = "birthdays"
)

// Failure wraps an infrastructure error with the stage it happened in. A missing contact
// is not a Failure: it is domain.ErrNotFound.
type Failure struct {
	Stage Stage
	Err   error
}

func (f *Failure) Error() string { return string(f.Stage) + ": " + f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

// UseCases is the application of the module.
type UseCases struct {
	contacts Contacts
	store    Store
	events   Events
}

// New builds the use cases from their ports.
func New(contacts Contacts, store Store, events Events) *UseCases {
	return &UseCases{contacts: contacts, store: store, events: events}
}

func (u *UseCases) load(ctx context.Context, actor domain.UserID, id domain.ContactID) (domain.Contact, error) {
	c, err := u.contacts.Get(ctx, actor, id)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return domain.Contact{}, &Failure{Stage: StageRetrieve, Err: err}
	}
	return c, err
}

// ShowContact returns a contact, complete or limited to fields.
func (u *UseCases) ShowContact(ctx context.Context, actor domain.UserID, id domain.ContactID, fields []string) (domain.Contact, error) {
	c, err := u.contacts.Show(ctx, actor, id, fields)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return domain.Contact{}, &Failure{Stage: StageRetrieve, Err: err}
	}
	return c, err
}

// CreateContact stores a new contact for the actor, then announces it.
func (u *UseCases) CreateContact(ctx context.Context, actor domain.UserID, draft domain.Contact, now time.Time) (domain.Contact, error) {
	draft.SyncPrimary()
	var created domain.Contact
	err := u.store.InTx(ctx, func(tx Tx) error {
		var err error
		created, err = tx.Create(ctx, actor, draft)
		return err
	})
	if err != nil {
		return domain.Contact{}, &Failure{Stage: StageCreate, Err: err}
	}
	u.events.ContactCreated(ctx, actor, created, now)
	return created, nil
}

// UpdateContact replaces the editable fields of a contact, then announces it.
func (u *UseCases) UpdateContact(ctx context.Context, actor domain.UserID, id domain.ContactID, edit domain.Contact, now time.Time) (domain.Contact, error) {
	var updated domain.Contact
	err := u.store.InTx(ctx, func(tx Tx) error {
		c, err := tx.Get(ctx, actor, id)
		if err != nil {
			return err
		}
		c.Replace(edit)
		updated, err = tx.Update(ctx, actor, c)
		return err
	})
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Contact{}, err
	}
	if err != nil {
		return domain.Contact{}, &Failure{Stage: StageUpdate, Err: err}
	}
	u.events.ContactUpdated(ctx, actor, updated, now)
	return updated, nil
}

// ArchiveContact archives a contact and deletes its reminders in one transaction.
func (u *UseCases) ArchiveContact(ctx context.Context, actor domain.UserID, id domain.ContactID) (domain.Contact, error) {
	c, err := u.load(ctx, actor, id)
	if err != nil {
		return domain.Contact{}, err
	}
	c.Archive()
	err = u.store.InTx(ctx, func(tx Tx) error {
		if err := tx.DeleteReminders(ctx, actor, id); err != nil {
			return err
		}
		return tx.SetArchived(ctx, actor, id, c.Archived)
	})
	if err != nil {
		return domain.Contact{}, &Failure{Stage: StageArchive, Err: err}
	}
	return c, nil
}

// UnarchiveContact brings an archived contact back.
func (u *UseCases) UnarchiveContact(ctx context.Context, actor domain.UserID, id domain.ContactID) (domain.Contact, error) {
	c, err := u.load(ctx, actor, id)
	if err != nil {
		return domain.Contact{}, err
	}
	c.Unarchive()
	err = u.store.InTx(ctx, func(tx Tx) error {
		return tx.SetArchived(ctx, actor, id, c.Archived)
	})
	if err != nil {
		return domain.Contact{}, &Failure{Stage: StageUnarchive, Err: err}
	}
	return c, nil
}

// DeleteContact moves a contact and its dependants to the trash, then announces it.
func (u *UseCases) DeleteContact(ctx context.Context, actor domain.UserID, id domain.ContactID, now time.Time) error {
	if _, err := u.load(ctx, actor, id); err != nil {
		return err
	}
	err := u.store.InTx(ctx, func(tx Tx) error {
		return tx.MoveToTrash(ctx, actor, id)
	})
	if err != nil {
		return &Failure{Stage: StageDelete, Err: err}
	}
	u.events.ContactDeleted(ctx, actor, id, now)
	return nil
}

// ListCircles returns the distinct circle names of the actor's contacts.
func (u *UseCases) ListCircles(ctx context.Context, actor domain.UserID) ([]string, error) {
	circles, err := u.contacts.Circles(ctx, actor)
	if err != nil {
		return nil, &Failure{Stage: StageCircles, Err: err}
	}
	return circles, nil
}

// ListContacts returns a page of the actor's contacts. Paging and sorting are normalised
// here, so every caller gets the same bounds.
func (u *UseCases) ListContacts(ctx context.Context, actor domain.UserID, q ListQuery) (ContactPage, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = DefaultLimit
	}
	q.Limit = min(q.Limit, MaxLimit)
	if q.Order != "asc" && q.Order != "desc" {
		q.Order = "desc"
	}
	contacts, total, err := u.contacts.List(ctx, actor, q)
	if err != nil {
		return ContactPage{}, &Failure{Stage: StageList, Err: err}
	}
	return ContactPage{Contacts: contacts, Total: total, Page: q.Page, Limit: q.Limit}, nil
}

func (u *UseCases) RandomContacts(ctx context.Context, actor domain.UserID) ([]domain.Contact, error) {
	contacts, err := u.contacts.Random(ctx, actor, 5)
	if err != nil {
		return nil, &Failure{Stage: StageList, Err: err}
	}
	return contacts, nil
}

func (u *UseCases) UpcomingBirthdays(ctx context.Context, actor domain.UserID, now time.Time) ([]domain.Birthday, error) {
	b, err := u.contacts.UpcomingBirthdays(ctx, actor, now)
	if err != nil {
		return nil, &Failure{Stage: StageBirthdays, Err: err}
	}
	return b, nil
}
