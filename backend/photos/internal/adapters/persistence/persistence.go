// Package persistence implements the ports of the photos module on top of GORM and the
// existing models. It is the only code of the module that imports gorm or models
// (docs/adr/0012-module-layout.md), and the place where every query is scoped to the actor.
package persistence

import (
	"context"
	"errors"

	"meerkat/models"
	"meerkat/photos/internal/app"
	"meerkat/photos/internal/domain"

	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

var (
	_ app.Contacts = (*Repository)(nil)
	_ app.Store    = (*Repository)(nil)
)

func (r *Repository) Photos(ctx context.Context, actor domain.UserID, id domain.ContactID, trashed bool) (domain.Photos, error) {
	return photos(r.db.WithContext(ctx), actor, id, trashed)
}

func (r *Repository) InTx(ctx context.Context, fn func(app.Tx) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&txRepo{tx: tx})
	})
}

type txRepo struct{ tx *gorm.DB }

func (t *txRepo) Photos(ctx context.Context, actor domain.UserID, id domain.ContactID, trashed bool) (domain.Photos, error) {
	return photos(t.tx.WithContext(ctx), actor, id, trashed)
}

func (t *txRepo) Replace(ctx context.Context, actor domain.UserID, id domain.ContactID, photo, thumbnail string) error {
	contact := models.Contact{Model: gorm.Model{ID: uint(id)}}
	return t.tx.WithContext(ctx).Model(&contact).Where("user_id = ?", uint(actor)).
		Updates(map[string]any{"photo": photo, "photo_thumbnail_file": thumbnail, "photo_thumbnail": ""}).Error
}

func photos(db *gorm.DB, actor domain.UserID, id domain.ContactID, trashed bool) (domain.Photos, error) {
	if trashed {
		db = db.Unscoped()
	}
	var m models.Contact
	err := db.Select("id", "photo", "photo_thumbnail_file", "photo_thumbnail").
		Where("user_id = ?", uint(actor)).First(&m, uint(id)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Photos{}, domain.ErrContactNotFound
	}
	if err != nil {
		return domain.Photos{}, err
	}
	return domain.Photos{ContactID: id, Photo: m.Photo, Thumbnail: m.PhotoThumbnailFile, Legacy: m.LegacyThumbnail}, nil
}
