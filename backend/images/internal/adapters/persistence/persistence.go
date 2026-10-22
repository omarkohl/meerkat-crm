// Package persistence implements the ports of the images module on top of GORM and the
// existing models. It is the only code of the module that imports gorm or models
// (docs/adr/0012-module-layout.md), and the place where every query is scoped to the actor.
package persistence

import (
	"context"
	"errors"
	"time"

	"meerkat/images/internal/app"
	"meerkat/images/internal/domain"
	"meerkat/models"
	"meerkat/services"

	"gorm.io/gorm"
)

type Repository struct {
	db  *gorm.DB
	dir string
}

func New(db *gorm.DB, dir string) *Repository { return &Repository{db: db, dir: dir} }

var _ app.Store = (*Repository)(nil)

func (r *Repository) Create(ctx context.Context, actor domain.UserID, s domain.Stored) (domain.Image, error) {
	m := models.Image{
		UserID: uint(actor), File: s.File, ThumbFile: s.ThumbFile, Mime: s.Mime,
		Width: s.Width, Height: s.Height, Bytes: s.Bytes, TakenAt: s.TakenAt,
	}
	if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
		return domain.Image{}, err
	}
	return toDomain(m), nil
}

func (r *Repository) Get(ctx context.Context, actor domain.UserID, id domain.ImageID) (domain.Image, error) {
	var m models.Image
	err := r.db.WithContext(ctx).Where("user_id = ?", uint(actor)).First(&m, uint(id)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Image{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Image{}, err
	}
	return toDomain(m), nil
}

func (r *Repository) DeletePendingBefore(ctx context.Context, t time.Time) (int, []string, error) {
	var rows []models.Image
	var names []string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		pending := tx.Where("contact_id IS NULL AND activity_id IS NULL AND note_id IS NULL AND created_at < ?", t)
		if err := pending.Select("id", "file", "thumb_file").Find(&rows).Error; err != nil {
			return err
		}
		for _, m := range rows {
			names = append(names, m.File, m.ThumbFile)
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Where("id IN ?", ids(rows)).Delete(&models.Image{}).Error
	})
	return len(rows), names, err
}

func (r *Repository) Files(ctx context.Context) (map[string]bool, error) {
	var rows []models.Image
	if err := r.db.WithContext(ctx).Select("file", "thumb_file").Find(&rows).Error; err != nil {
		return nil, err
	}
	keep := make(map[string]bool, 2*len(rows))
	for _, m := range rows {
		keep[m.File], keep[m.ThumbFile] = true, true
	}
	return keep, nil
}

func (r *Repository) SetContactImages(ctx context.Context, actor domain.UserID, contact domain.ContactID, ids []uint) ([]domain.Image, error) {
	eff := &services.ImageEffects{}
	var rows []models.Image
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := services.SetContactImages(tx, r.dir, uint(actor), uint(contact), ids, eff); err != nil {
			return err
		}
		return tx.Where("user_id = ? AND contact_id = ?", uint(actor), uint(contact)).Order("position, id").Find(&rows).Error
	})
	eff.Finish(r.dir, err)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]domain.Image, len(rows))
	for i, m := range rows {
		out[i] = toDomain(m)
	}
	return out, nil
}

func (r *Repository) SetMainImage(ctx context.Context, actor domain.UserID, contact domain.ContactID, image domain.ImageID, crop domain.Crop) (domain.Image, error) {
	eff := &services.ImageEffects{}
	var m models.Image
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		in := services.ImageCropInput{X: crop.X, Y: crop.Y, Size: crop.Size}
		if err := services.SetMainImage(tx, r.dir, uint(actor), uint(contact), uint(image), in, eff); err != nil {
			return err
		}
		return tx.Where("user_id = ?", uint(actor)).First(&m, uint(image)).Error
	})
	eff.Finish(r.dir, err)
	if err != nil {
		return domain.Image{}, mapError(err)
	}
	return toDomain(m), nil
}

func mapError(err error) error {
	switch {
	case errors.Is(err, services.ErrContactNotFound):
		return domain.ErrContactNotFound
	case errors.Is(err, services.ErrImageNotFound):
		return domain.ErrNotFound
	case errors.Is(err, services.ErrTooManyImages):
		return domain.ErrTooManyImages
	case errors.Is(err, services.ErrInvalidCrop):
		return domain.ErrInvalidCrop
	}
	return err
}

func ids(rows []models.Image) []uint {
	out := make([]uint, len(rows))
	for i, m := range rows {
		out[i] = m.ID
	}
	return out
}

func toDomain(m models.Image) domain.Image {
	img := domain.Image{ID: domain.ImageID(m.ID), Stored: domain.Stored{
		File: m.File, ThumbFile: m.ThumbFile, Mime: m.Mime, Width: m.Width, Height: m.Height, Bytes: m.Bytes, TakenAt: m.TakenAt,
	}}
	if ref := m.Ref(); ref.Crop != nil {
		img.Crop = &domain.Crop{X: ref.Crop.X, Y: ref.Crop.Y, Size: ref.Crop.Size}
	}
	return img
}
