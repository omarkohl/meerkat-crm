// Package persistence implements the ports of the notes module on top of GORM and the
// existing models. It is the only code of the module that imports gorm or models
// (docs/adr/0012-module-layout.md), and the place where every query is scoped to the actor.
package persistence

import (
	"context"
	"errors"

	"meerkat/models"
	"meerkat/notes/internal/app"
	"meerkat/notes/internal/domain"
	"meerkat/search"
	"meerkat/services"

	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

var (
	_ app.Notes = (*Repository)(nil)
	_ app.Store = (*Repository)(nil)
)

func first(db *gorm.DB, actor domain.UserID, id domain.NoteID) (domain.Note, error) {
	var m models.Note
	err := db.Where("user_id = ?", uint(actor)).First(&m, uint(id)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Note{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Note{}, err
	}
	notes := []domain.Note{toDomain(m)}
	if err := withImages(db, actor, notes); err != nil {
		return domain.Note{}, err
	}
	return notes[0], nil
}

func (r *Repository) Get(ctx context.Context, actor domain.UserID, id domain.NoteID) (domain.Note, error) {
	return first(r.db.WithContext(ctx), actor, id)
}

func (r *Repository) ForContact(ctx context.Context, actor domain.UserID, contact domain.ContactID) ([]domain.Note, error) {
	db := r.db.WithContext(ctx)
	if ok, err := contactExists(db, actor, contact); err != nil {
		return nil, err
	} else if !ok {
		return nil, domain.ErrContactNotFound
	}
	var found []models.Note
	if err := db.Where("user_id = ? AND contact_id = ?", uint(actor), uint(contact)).Order("id").Find(&found).Error; err != nil {
		return nil, err
	}
	notes := toDomainList(found)
	return notes, withImages(db, actor, notes)
}

func (r *Repository) Unassigned(ctx context.Context, actor domain.UserID, f app.Filter) ([]domain.Note, int64, error) {
	db := r.db.WithContext(ctx)
	base := db.Model(&models.Note{}).Where("notes.user_id = ? AND contact_id IS NULL", uint(actor))
	if f.From != nil {
		base = base.Where("notes.date >= ?", *f.From)
	}
	if f.To != nil {
		base = base.Where("notes.date < ?", *f.To)
	}
	if f.Search != "" {
		if match, ok := search.MatchQuery(f.Search); ok && search.Available(db, "notes_fts") {
			base = base.Where("notes.id IN (SELECT rowid FROM notes_fts WHERE notes_fts MATCH ?)", match)
		} else {
			base = base.Where("LOWER(content) LIKE ?", "%"+f.Search+"%")
		}
	}
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var found []models.Note
	if err := base.Session(&gorm.Session{}).Order("notes.date DESC, notes.id DESC").
		Limit(f.Limit).Offset((f.Page - 1) * f.Limit).Find(&found).Error; err != nil {
		return nil, 0, err
	}
	notes := toDomainList(found)
	return notes, total, withImages(db, actor, notes)
}

// withImages fills the images of notes with one query.
func withImages(db *gorm.DB, actor domain.UserID, notes []domain.Note) error {
	ids := make([]uint, len(notes))
	for i, n := range notes {
		ids[i] = uint(n.ID)
	}
	byNote, err := services.ImagesFor(db, uint(actor), services.ImageOfNote, ids)
	if err != nil {
		return err
	}
	for i := range notes {
		notes[i].Images = imageRefs(byNote[uint(notes[i].ID)])
	}
	return nil
}

func imageRefs(refs []models.ImageRef) []domain.ImageRef {
	out := make([]domain.ImageRef, len(refs))
	for i, r := range refs {
		out[i] = domain.ImageRef{
			ID: r.ID, Width: r.Width, Height: r.Height, Mime: r.Mime, Bytes: r.Bytes, TakenAt: r.TakenAt,
			URL: r.URL, ThumbURL: r.ThumbURL,
		}
	}
	return out
}

func (r *Repository) InTx(ctx context.Context, fn func(app.Tx) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&txRepo{tx: tx})
	})
}

type txRepo struct{ tx *gorm.DB }

func contactExists(db *gorm.DB, actor domain.UserID, id domain.ContactID) (bool, error) {
	var n int64
	err := db.Model(&models.Contact{}).Where("user_id = ? AND id = ?", uint(actor), uint(id)).Count(&n).Error
	return n > 0, err
}

func (t *txRepo) Get(ctx context.Context, actor domain.UserID, id domain.NoteID) (domain.Note, error) {
	return first(t.tx.WithContext(ctx), actor, id)
}

func (t *txRepo) ContactExists(ctx context.Context, actor domain.UserID, id domain.ContactID) (bool, error) {
	return contactExists(t.tx.WithContext(ctx), actor, id)
}

func (t *txRepo) Create(ctx context.Context, actor domain.UserID, n domain.Note) (domain.Note, error) {
	m := models.Note{UserID: uint(actor), Content: n.Content, Date: n.Date, ContactID: contactID(n.ContactID)}
	if err := t.tx.WithContext(ctx).Create(&m).Error; err != nil {
		return domain.Note{}, err
	}
	created := toDomain(m)
	created.Images = []domain.ImageRef{}
	return created, nil
}

func (t *txRepo) SetImages(ctx context.Context, actor domain.UserID, id domain.NoteID, ids []uint) ([]domain.ImageRef, []string, error) {
	db := t.tx.WithContext(ctx)
	removed, err := services.SetOwnerImages(db, uint(actor), services.ImageOwner{Kind: services.ImageOfNote, ID: uint(id)}, ids)
	switch {
	case errors.Is(err, services.ErrImageNotFound):
		return nil, nil, domain.ErrImageNotFound
	case errors.Is(err, services.ErrTooManyImages):
		return nil, nil, domain.ErrTooManyImages
	case err != nil:
		return nil, nil, err
	}
	byNote, err := services.ImagesFor(db, uint(actor), services.ImageOfNote, []uint{uint(id)})
	return imageRefs(byNote[uint(id)]), removed, err
}

func (t *txRepo) DeleteImages(ctx context.Context, actor domain.UserID, id domain.NoteID) ([]string, error) {
	return services.DeleteOwnerImages(t.tx.WithContext(ctx), uint(actor), services.ImageOwner{Kind: services.ImageOfNote, ID: uint(id)})
}

func (t *txRepo) Update(ctx context.Context, actor domain.UserID, n domain.Note) (domain.Note, error) {
	m := models.Note{
		Model:     gorm.Model{ID: uint(n.ID), CreatedAt: n.CreatedAt},
		UserID:    uint(actor),
		Content:   n.Content,
		Date:      n.Date,
		ContactID: contactID(n.ContactID),
	}
	res := t.tx.WithContext(ctx).Model(&m).Where("user_id = ?", uint(actor)).
		Select("content", "date", "contact_id").Updates(&m)
	if res.Error != nil {
		return domain.Note{}, res.Error
	}
	if res.RowsAffected == 0 {
		return domain.Note{}, domain.ErrNotFound
	}
	return toDomain(m), nil
}

func (t *txRepo) Delete(ctx context.Context, actor domain.UserID, id domain.NoteID) error {
	return t.tx.WithContext(ctx).Where("user_id = ?", uint(actor)).Delete(&models.Note{}, uint(id)).Error
}

func contactID(id *domain.ContactID) *uint {
	if id == nil {
		return nil
	}
	v := uint(*id)
	return &v
}

func toDomainList(found []models.Note) []domain.Note {
	out := make([]domain.Note, len(found))
	for i, m := range found {
		out[i] = toDomain(m)
	}
	return out
}

func toDomain(m models.Note) domain.Note {
	n := domain.Note{
		ID: domain.NoteID(m.ID), UserID: domain.UserID(m.UserID), CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
		Content: m.Content, Date: m.Date,
	}
	if m.DeletedAt.Valid {
		t := m.DeletedAt.Time
		n.DeletedAt = &t
	}
	if m.ContactID != nil {
		id := domain.ContactID(*m.ContactID)
		n.ContactID = &id
	}
	return n
}
