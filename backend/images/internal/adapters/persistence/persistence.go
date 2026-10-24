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

const relatedSQL = `
SELECT i.id AS image_id, 'note' AS kind, n.id AS source_id, n.content AS label, n.date AS date, i.position AS position
FROM images i JOIN notes n ON n.id = i.note_id AND n.deleted_at IS NULL
WHERE i.user_id = @user AND n.user_id = @user AND n.contact_id = @contact
UNION ALL
SELECT i.id, 'activity', a.id, a.title, a.date, i.position
FROM images i JOIN activities a ON a.id = i.activity_id AND a.deleted_at IS NULL
JOIN activity_contacts ac ON ac.activity_id = a.id AND ac.contact_id = @contact
WHERE i.user_id = @user AND a.user_id = @user
ORDER BY date DESC, source_id DESC, position, image_id
LIMIT @limit OFFSET @offset`

const relatedCountSQL = `
SELECT
  (SELECT COUNT(*) FROM images i JOIN notes n ON n.id = i.note_id AND n.deleted_at IS NULL
    WHERE i.user_id = @user AND n.user_id = @user AND n.contact_id = @contact) +
  (SELECT COUNT(*) FROM images i JOIN activities a ON a.id = i.activity_id AND a.deleted_at IS NULL
    JOIN activity_contacts ac ON ac.activity_id = a.id AND ac.contact_id = @contact
    WHERE i.user_id = @user AND a.user_id = @user)`

const relatedLabelRunes = 80

func (r *Repository) Related(ctx context.Context, actor domain.UserID, contact domain.ContactID, offset, limit int) ([]domain.Related, int64, error) {
	db := r.db.WithContext(ctx)
	var owned int64
	if err := db.Model(&models.Contact{}).Where("user_id = ? AND id = ?", uint(actor), uint(contact)).Count(&owned).Error; err != nil {
		return nil, 0, err
	}
	if owned == 0 {
		return nil, 0, domain.ErrContactNotFound
	}
	args := map[string]any{"user": uint(actor), "contact": uint(contact), "limit": limit, "offset": offset}
	var rows []struct {
		ImageID  uint
		Kind     string
		SourceID uint
		Label    string
		Date     time.Time
	}
	if err := db.Raw(relatedSQL, args).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	var total int64
	if err := db.Raw(relatedCountSQL, args).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	want := make([]uint, len(rows))
	for i, row := range rows {
		want[i] = row.ImageID
	}
	var found []models.Image
	if len(want) > 0 {
		if err := db.Where("user_id = ? AND id IN ?", uint(actor), want).Find(&found).Error; err != nil {
			return nil, 0, err
		}
	}
	byID := make(map[uint]models.Image, len(found))
	for _, m := range found {
		byID[m.ID] = m
	}
	out := make([]domain.Related, 0, len(rows))
	for _, row := range rows {
		m, ok := byID[row.ImageID]
		if !ok {
			continue
		}
		label := []rune(row.Label)
		if len(label) > relatedLabelRunes {
			label = append(label[:relatedLabelRunes], '…')
		}
		out = append(out, domain.Related{Image: toDomain(m), Source: domain.RelatedSource{
			Kind: row.Kind, ID: row.SourceID, Label: string(label), Date: row.Date,
		}})
	}
	return out, total, nil
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
