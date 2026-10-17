// Package app holds the use cases of the users module. It depends on ports (the interfaces
// below) and on the domain, never on GORM, Gin or huma (docs/adr/0012-module-layout.md).
package app

import (
	"context"
	"errors"

	"meerkat/users/internal/domain"
)

// Preferences reads and writes one user's settings, one at a time: a write never touches the
// password or any other column.
type Preferences interface {
	CustomFieldNames(ctx context.Context, actor domain.UserID) ([]string, error)
	EnabledContactFields(ctx context.Context, actor domain.UserID) ([]string, error)
	SetLanguage(ctx context.Context, actor domain.UserID, language string) error
	SetDateFormat(ctx context.Context, actor domain.UserID, format string) error
	SetCustomFieldNames(ctx context.Context, actor domain.UserID, names []string) error
	SetEnabledContactFields(ctx context.Context, actor domain.UserID, fields []string) error
}

type Stage string

const (
	StageQuery  Stage = "query"
	StageUpdate Stage = "update"
)

// Failure wraps an infrastructure error with the stage it happened in. A missing user and a
// value that is not supported are domain errors.
type Failure struct {
	Stage Stage
	Err   error
}

func (f *Failure) Error() string { return string(f.Stage) + ": " + f.Err.Error() }
func (f *Failure) Unwrap() error { return f.Err }

type UseCases struct {
	prefs     Preferences
	languages []string
}

func New(prefs Preferences, supportedLanguages []string) *UseCases {
	return &UseCases{prefs: prefs, languages: supportedLanguages}
}

func stage(s Stage, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, domain.ErrUserNotFound) {
		return err
	}
	return &Failure{Stage: s, Err: err}
}

func (u *UseCases) SetLanguage(ctx context.Context, actor domain.UserID, language string) error {
	if err := domain.ValidateLanguage(language, u.languages); err != nil {
		return err
	}
	return stage(StageUpdate, u.prefs.SetLanguage(ctx, actor, language))
}

func (u *UseCases) SetDateFormat(ctx context.Context, actor domain.UserID, format string) error {
	if err := domain.ValidateDateFormat(format); err != nil {
		return err
	}
	return stage(StageUpdate, u.prefs.SetDateFormat(ctx, actor, format))
}

// CustomFieldNames returns the user's names, an empty list when none were set.
func (u *UseCases) CustomFieldNames(ctx context.Context, actor domain.UserID) ([]string, error) {
	names, err := u.prefs.CustomFieldNames(ctx, actor)
	if err != nil {
		return nil, stage(StageQuery, err)
	}
	if names == nil {
		names = []string{}
	}
	return names, nil
}

func (u *UseCases) SetCustomFieldNames(ctx context.Context, actor domain.UserID, names []string) error {
	return stage(StageUpdate, u.prefs.SetCustomFieldNames(ctx, actor, names))
}

// EnabledContactFields returns the user's fields, nil when never configured so the client
// applies its defaults.
func (u *UseCases) EnabledContactFields(ctx context.Context, actor domain.UserID) ([]string, error) {
	fields, err := u.prefs.EnabledContactFields(ctx, actor)
	return fields, stage(StageQuery, err)
}

func (u *UseCases) SetEnabledContactFields(ctx context.Context, actor domain.UserID, fields []string) error {
	return stage(StageUpdate, u.prefs.SetEnabledContactFields(ctx, actor, fields))
}
