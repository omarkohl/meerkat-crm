// Package app holds the use case of the global search. It depends on a port and on the domain,
// never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"errors"
	"strings"

	"meerkat/globalsearch/internal/domain"
)

var (
	ErrEmptyTerm   = errors.New("search term is empty")
	ErrUnknownKind = errors.New("unknown kind of result")
)

type Page[T any] struct {
	Total int
	Items []T
	// Substring says the match was a substring search (digits, Han/Kana/Hangul text, or no
	// index), which decides how the matched text is marked.
	Substring bool
}

// Index finds the actor's rows that match a term, best first. Trashed rows are not found.
type Index interface {
	Contacts(ctx context.Context, actor domain.UserID, term string, limit, offset int) (Page[domain.Contact], error)
	Notes(ctx context.Context, actor domain.UserID, term string, limit, offset int) (Page[domain.Note], error)
	Activities(ctx context.Context, actor domain.UserID, term string, limit, offset int) (Page[domain.Activity], error)
}

type Group[T any] struct {
	Total int
	Items []T
}

// Request asks for one kind of result, or for all of them when Kind is empty.
type Request struct {
	Term   string
	Kind   domain.Kind
	Limit  int
	Offset int
}

// Result has the groups that were asked for; the others are nil.
type Result struct {
	Term       string
	Contacts   *Group[domain.ContactHit]
	Notes      *Group[domain.NoteHit]
	Activities *Group[domain.ActivityHit]
}

type Failure struct {
	Kind domain.Kind
	Err  error
}

func (f *Failure) Error() string { return "search " + string(f.Kind) + ": " + f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

type UseCases struct{ index Index }

func New(index Index) *UseCases { return &UseCases{index: index} }

func (u *UseCases) Search(ctx context.Context, actor domain.UserID, req Request) (Result, error) {
	term := strings.TrimSpace(req.Term)
	if term == "" {
		return Result{}, ErrEmptyTerm
	}
	switch req.Kind {
	case "", domain.KindContacts, domain.KindNotes, domain.KindActivities:
	default:
		return Result{}, ErrUnknownKind
	}
	res := Result{Term: term}
	want := func(k domain.Kind) bool { return req.Kind == "" || req.Kind == k }
	if want(domain.KindContacts) {
		page, err := u.index.Contacts(ctx, actor, term, req.Limit, req.Offset)
		if err != nil {
			return Result{}, &Failure{Kind: domain.KindContacts, Err: err}
		}
		g := &Group[domain.ContactHit]{Total: page.Total, Items: []domain.ContactHit{}}
		for _, c := range page.Items {
			g.Items = append(g.Items, domain.HitContact(c, term, page.Substring))
		}
		res.Contacts = g
	}
	if want(domain.KindNotes) {
		page, err := u.index.Notes(ctx, actor, term, req.Limit, req.Offset)
		if err != nil {
			return Result{}, &Failure{Kind: domain.KindNotes, Err: err}
		}
		g := &Group[domain.NoteHit]{Total: page.Total, Items: []domain.NoteHit{}}
		for _, n := range page.Items {
			g.Items = append(g.Items, domain.HitNote(n, term, page.Substring))
		}
		res.Notes = g
	}
	if want(domain.KindActivities) {
		page, err := u.index.Activities(ctx, actor, term, req.Limit, req.Offset)
		if err != nil {
			return Result{}, &Failure{Kind: domain.KindActivities, Err: err}
		}
		g := &Group[domain.ActivityHit]{Total: page.Total, Items: []domain.ActivityHit{}}
		for _, a := range page.Items {
			g.Items = append(g.Items, domain.HitActivity(a, term, page.Substring))
		}
		res.Activities = g
	}
	return res, nil
}
