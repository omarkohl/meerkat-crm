// Package persistence implements the ports of the users module on top of GORM and the
// existing models. It is the only code of the module that imports gorm or models
// (docs/adr/0012-module-layout.md), and the place where every query is scoped to the actor.
package persistence

import (
	"context"
	"errors"

	"meerkat/models"
	"meerkat/users/internal/app"
	"meerkat/users/internal/domain"

	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

var _ app.Preferences = (*Repository)(nil)

func (r *Repository) user(ctx context.Context, actor domain.UserID) (models.User, error) {
	var u models.User
	err := r.db.WithContext(ctx).Select("id", "custom_field_names", "enabled_contact_fields").First(&u, uint(actor)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return u, domain.ErrUserNotFound
	}
	return u, err
}

func (r *Repository) CustomFieldNames(ctx context.Context, actor domain.UserID) ([]string, error) {
	u, err := r.user(ctx, actor)
	return u.CustomFieldNames, err
}

func (r *Repository) EnabledContactFields(ctx context.Context, actor domain.UserID) ([]string, error) {
	u, err := r.user(ctx, actor)
	return u.EnabledContactFields, err
}

// set writes one column of the user and nothing else.
func (r *Repository) set(ctx context.Context, actor domain.UserID, column string, values models.User) error {
	u := models.User{Model: gorm.Model{ID: uint(actor)}}
	res := r.db.WithContext(ctx).Model(&u).Select(column).Updates(&values)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}

func (r *Repository) SetLanguage(ctx context.Context, actor domain.UserID, language string) error {
	return r.set(ctx, actor, "Language", models.User{Language: language})
}

func (r *Repository) SetDateFormat(ctx context.Context, actor domain.UserID, format string) error {
	return r.set(ctx, actor, "DateFormat", models.User{DateFormat: format})
}

func (r *Repository) SetCustomFieldNames(ctx context.Context, actor domain.UserID, names []string) error {
	return r.set(ctx, actor, "CustomFieldNames", models.User{CustomFieldNames: names})
}

func (r *Repository) SetEnabledContactFields(ctx context.Context, actor domain.UserID, fields []string) error {
	return r.set(ctx, actor, "EnabledContactFields", models.User{EnabledContactFields: fields})
}
