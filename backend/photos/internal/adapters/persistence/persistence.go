// Package persistence implements the ports of the photos module on top of GORM and the
// existing models. It is the only code of the module that imports gorm or models
// (docs/adr/0012-module-layout.md), and the place where every query is scoped to the actor.
package persistence

import (
	"context"
	"errors"

	"meerkat/imageproc"
	"meerkat/models"
	"meerkat/photos/internal/app"
	"meerkat/photos/internal/domain"
	"meerkat/services"

	"gorm.io/gorm"
)

type Repository struct {
	db  *gorm.DB
	dir string
}

func New(db *gorm.DB, dir string) *Repository { return &Repository{db: db, dir: dir} }

var _ app.Contacts = (*Repository)(nil)

func (r *Repository) Photos(ctx context.Context, actor domain.UserID, id domain.ContactID, trashed bool) (domain.Photos, error) {
	return photos(r.db.WithContext(ctx), actor, id, trashed)
}

func (r *Repository) AddPhoto(ctx context.Context, actor domain.UserID, id domain.ContactID, data []byte) (domain.Photos, error) {
	img, err := services.StoreImage(ctx, r.dir, uint(actor), data)
	if err != nil {
		if errors.Is(err, imageproc.ErrUnsupported) || errors.Is(err, imageproc.ErrInvalid) || errors.Is(err, imageproc.ErrTooLarge) {
			return domain.Photos{}, errors.Join(domain.ErrInvalidPhoto, err)
		}
		return domain.Photos{}, err
	}
	eff := &services.ImageEffects{}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := services.AttachContactImage(tx, r.dir, uint(actor), uint(id), img, false, eff)
		return err
	})
	eff.Finish(r.dir, err)
	switch {
	case errors.Is(err, services.ErrContactNotFound):
		return domain.Photos{}, domain.ErrContactNotFound
	case errors.Is(err, services.ErrTooManyImages):
		return domain.Photos{}, domain.ErrTooManyImages
	case err != nil:
		return domain.Photos{}, err
	}
	return photos(r.db.WithContext(ctx), actor, id, false)
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
