// Package app holds the use cases of the exports module. It depends on ports (the interfaces
// below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"time"

	"meerkat/exports/internal/domain"
)

// Source reads everything of the actor that the CSV export contains.
type Source interface {
	Dataset(ctx context.Context, actor domain.UserID) (domain.Dataset, error)
}

// Cards writes the actor's contacts, with photos, as vCards.
type Cards interface {
	Encode(ctx context.Context, actor domain.UserID) (Encoded, error)
}

// Encoded is the vCard file; Skipped are contacts that could not be encoded and are not in it.
type Encoded struct {
	Body     []byte
	Contacts int
	Skipped  []uint
}

type Audit interface {
	Record(ctx context.Context, actor domain.UserID, event, detail string, client domain.Client)
}

type FetchError struct {
	What string
	Err  error
}

func (e *FetchError) Error() string { return "fetch " + e.What + ": " + e.Err.Error() }
func (e *FetchError) Unwrap() error { return e.Err }

type GenerateError struct{ Err error }

func (e *GenerateError) Error() string { return "generate export: " + e.Err.Error() }
func (e *GenerateError) Unwrap() error { return e.Err }

type Document struct {
	Filename    string
	ContentType string
	Body        []byte
	Contacts    int
	Activities  int
	Notes       int
	Reminders   int
	Skipped     []uint
}

type UseCases struct {
	source Source
	cards  Cards
	audit  Audit
}

func New(source Source, cards Cards, audit Audit) *UseCases {
	return &UseCases{source: source, cards: cards, audit: audit}
}

func (u *UseCases) ExportCSV(ctx context.Context, actor domain.UserID, now time.Time, client domain.Client) (Document, error) {
	data, err := u.source.Dataset(ctx, actor)
	if err != nil {
		return Document{}, err
	}
	body, err := domain.RenderCSV(data)
	if err != nil {
		return Document{}, &GenerateError{Err: err}
	}
	u.audit.Record(ctx, actor, domain.EventExport, "format:csv", client)
	return Document{Filename: domain.Filename("meerkat-export", "csv", now), ContentType: domain.CSVType, Body: body, Contacts: len(data.Contacts), Activities: len(data.Activities), Notes: len(data.Notes), Reminders: len(data.Reminders)}, nil
}

func (u *UseCases) ExportVCF(ctx context.Context, actor domain.UserID, now time.Time, client domain.Client) (Document, error) {
	enc, err := u.cards.Encode(ctx, actor)
	if err != nil {
		return Document{}, err
	}
	u.audit.Record(ctx, actor, domain.EventExport, "format:vcf", client)
	return Document{Filename: domain.Filename("meerkat-contacts", "vcf", now), ContentType: domain.VCardType, Body: enc.Body, Contacts: enc.Contacts, Skipped: enc.Skipped}, nil
}
