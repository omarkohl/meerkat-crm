// Package app holds the use cases of the imports module. It depends on ports (the interfaces
// below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"io"

	"meerkat/imports/internal/domain"

	"github.com/rs/zerolog"
)

// Wizard is the import wizard: parsing, the in-memory sessions of the steps, and applying the
// chosen actions. Its errors may carry their own HTTP status; the HTTP adapter passes those on.
type Wizard interface {
	MaxSize(kind domain.Kind) int64
	UploadCSV(ctx context.Context, actor domain.UserID, file io.Reader) (domain.Upload, error)
	UploadVCF(ctx context.Context, actor domain.UserID, file io.Reader) (domain.Preview, error)
	Preview(ctx context.Context, actor domain.UserID, session string, mappings []domain.Mapping) (domain.Preview, error)
	ConfirmCSV(ctx context.Context, actor domain.UserID, session string, actions []domain.Action, log *zerolog.Logger) (domain.Result, error)
	ConfirmVCF(ctx context.Context, actor domain.UserID, session string, actions []domain.Action, log *zerolog.Logger) (domain.Result, error)
	PhotoStatus(actor domain.UserID, session string) (domain.PhotoStatus, error)
}

type UseCases struct{ wizard Wizard }

func New(wizard Wizard) *UseCases { return &UseCases{wizard: wizard} }

type File struct {
	Name    string
	Size    int64
	Content io.Reader
}

func (u *UseCases) check(kind domain.Kind, f *File) error {
	if f == nil {
		return domain.NoFileError{}
	}
	return domain.CheckFile(kind, f.Name, f.Size, u.wizard.MaxSize(kind))
}

func (u *UseCases) UploadCSV(ctx context.Context, actor domain.UserID, f *File) (domain.Upload, error) {
	if err := u.check(domain.CSV, f); err != nil {
		return domain.Upload{}, err
	}
	return u.wizard.UploadCSV(ctx, actor, f.Content)
}

func (u *UseCases) UploadVCF(ctx context.Context, actor domain.UserID, f *File) (domain.Preview, error) {
	if err := u.check(domain.VCF, f); err != nil {
		return domain.Preview{}, err
	}
	return u.wizard.UploadVCF(ctx, actor, f.Content)
}

func (u *UseCases) Preview(ctx context.Context, actor domain.UserID, session string, mappings []domain.Mapping) (domain.Preview, error) {
	return u.wizard.Preview(ctx, actor, session, mappings)
}

func (u *UseCases) ConfirmCSV(ctx context.Context, actor domain.UserID, session string, actions []domain.Action, log *zerolog.Logger) (domain.Result, error) {
	return u.wizard.ConfirmCSV(ctx, actor, session, actions, log)
}

func (u *UseCases) ConfirmVCF(ctx context.Context, actor domain.UserID, session string, actions []domain.Action, log *zerolog.Logger) (domain.Result, error) {
	return u.wizard.ConfirmVCF(ctx, actor, session, actions, log)
}

func (u *UseCases) PhotoStatus(actor domain.UserID, session string) (domain.PhotoStatus, error) {
	return u.wizard.PhotoStatus(actor, session)
}
