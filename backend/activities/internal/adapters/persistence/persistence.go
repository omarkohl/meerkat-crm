// Package persistence implements the ports of the activities module on top of GORM and the
// existing models. It is the only code of the module that imports gorm or models
// (docs/adr/0012-module-layout.md), and the place where every query is scoped to the actor.
package persistence

import (
	"context"
	"errors"

	"meerkat/activities/internal/app"
	"meerkat/activities/internal/domain"
	"meerkat/models"
	"meerkat/search"

	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

var (
	_ app.Activities = (*Repository)(nil)
	_ app.Store      = (*Repository)(nil)
)

func (r *Repository) Get(ctx context.Context, actor domain.UserID, id domain.ActivityID) (domain.Activity, error) {
	return get(r.db.WithContext(ctx), actor, id)
}

func (r *Repository) ForContact(ctx context.Context, actor domain.UserID, contact domain.ContactID) ([]domain.Activity, error) {
	db := r.db.WithContext(ctx)
	var n int64
	if err := db.Model(&models.Contact{}).Where("user_id = ? AND id = ?", uint(actor), uint(contact)).Count(&n).Error; err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, domain.ErrContactNotFound
	}
	var found []models.Activity
	err := db.Where("activities.user_id = ?", uint(actor)).
		Joins("JOIN activity_contacts ON activities.id = activity_contacts.activity_id").
		Where("activity_contacts.contact_id = ?", uint(contact)).
		Order("activities.id").Find(&found).Error
	if err != nil {
		return nil, err
	}
	return withContacts(db, actor, found)
}

func (r *Repository) List(ctx context.Context, actor domain.UserID, f app.Filter) ([]domain.Activity, int64, error) {
	db := r.db.WithContext(ctx)
	base := db.Model(&models.Activity{}).Where("activities.user_id = ?", uint(actor))
	if f.From != nil {
		base = base.Where("COALESCE(activities.end_date, activities.date) >= ?", *f.From)
	}
	if f.To != nil {
		base = base.Where("activities.date < ?", *f.To)
	}
	if f.Search != "" {
		base = base.Select("DISTINCT activities.*").
			Joins("LEFT JOIN activity_contacts ON activity_contacts.activity_id = activities.id").
			Joins("LEFT JOIN contacts ON contacts.id = activity_contacts.contact_id AND contacts.user_id = ? AND contacts.deleted_at IS NULL", uint(actor))
		if match, ok := search.MatchQuery(f.Search); ok && search.Available(db, "activities_fts") && search.Available(db, "contacts_fts") {
			base = base.Where(
				"activities.id IN (SELECT rowid FROM activities_fts WHERE activities_fts MATCH ?) OR contacts.id IN (SELECT rowid FROM contacts_fts WHERE contacts_fts MATCH ?)",
				match, search.ContactNames(match))
		} else {
			like := "%" + f.Search + "%"
			base = base.Where(db.Where("LOWER(activities.title) LIKE ?", like).
				Or("LOWER(activities.description) LIKE ?", like).
				Or("LOWER(activities.location) LIKE ?", like).
				Or("LOWER(COALESCE(contacts.firstname, '')) LIKE ?", like).
				Or("LOWER(COALESCE(contacts.lastname, '')) LIKE ?", like).
				Or("LOWER(COALESCE(contacts.nickname, '')) LIKE ?", like))
		}
	}
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var found []models.Activity
	err := base.Session(&gorm.Session{}).Order("activities.date DESC, activities.id DESC").
		Limit(f.Limit).Offset((f.Page - 1) * f.Limit).Find(&found).Error
	if err != nil {
		return nil, 0, err
	}
	if !f.WithContacts {
		out := make([]domain.Activity, len(found))
		for i, m := range found {
			out[i] = toDomain(m, nil)
		}
		return out, total, nil
	}
	out, err := withContacts(db, actor, found)
	return out, total, err
}

func (r *Repository) InTx(ctx context.Context, fn func(app.Tx) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&txRepo{tx: tx})
	})
}

type txRepo struct{ tx *gorm.DB }

func (t *txRepo) Get(ctx context.Context, actor domain.UserID, id domain.ActivityID) (domain.Activity, error) {
	return get(t.tx.WithContext(ctx), actor, id)
}

func (t *txRepo) CountContacts(ctx context.Context, actor domain.UserID, ids []domain.ContactID) (int, error) {
	var n int64
	err := t.tx.WithContext(ctx).Model(&models.Contact{}).
		Where("user_id = ? AND id IN ?", uint(actor), contactIDs(ids)).Count(&n).Error
	return int(n), err
}

func (t *txRepo) Create(ctx context.Context, actor domain.UserID, a domain.Activity, contacts []domain.ContactID) (domain.Activity, error) {
	db := t.tx.WithContext(ctx)
	m := models.Activity{UserID: uint(actor), Title: a.Title, Description: a.Description, Location: a.Location, Date: a.Date, EndDate: a.EndDate}
	if err := db.Create(&m).Error; err != nil {
		return domain.Activity{}, err
	}
	if err := link(db, m.ID, contacts); err != nil {
		return domain.Activity{}, err
	}
	return one(db, actor, m)
}

func (t *txRepo) Update(ctx context.Context, actor domain.UserID, a domain.Activity, contacts []domain.ContactID) (domain.Activity, error) {
	db := t.tx.WithContext(ctx)
	m := models.Activity{
		Model: gorm.Model{ID: uint(a.ID), CreatedAt: a.CreatedAt}, UserID: uint(actor),
		Title: a.Title, Description: a.Description, Location: a.Location, Date: a.Date, EndDate: a.EndDate,
	}
	res := db.Model(&m).Where("user_id = ?", uint(actor)).
		Select("title", "description", "location", "date", "end_date").Updates(&m)
	if res.Error != nil {
		return domain.Activity{}, res.Error
	}
	if res.RowsAffected == 0 {
		return domain.Activity{}, domain.ErrNotFound
	}
	if contacts != nil {
		if err := db.Exec("DELETE FROM activity_contacts WHERE activity_id = ?", m.ID).Error; err != nil {
			return domain.Activity{}, err
		}
		if err := link(db, m.ID, contacts); err != nil {
			return domain.Activity{}, err
		}
	}
	return one(db, actor, m)
}

func (t *txRepo) Delete(ctx context.Context, actor domain.UserID, id domain.ActivityID) error {
	return t.tx.WithContext(ctx).Where("user_id = ?", uint(actor)).Delete(&models.Activity{}, uint(id)).Error
}

func get(db *gorm.DB, actor domain.UserID, id domain.ActivityID) (domain.Activity, error) {
	var m models.Activity
	err := db.Where("user_id = ?", uint(actor)).First(&m, uint(id)).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Activity{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Activity{}, err
	}
	return one(db, actor, m)
}

func link(db *gorm.DB, activity uint, contacts []domain.ContactID) error {
	if len(contacts) == 0 {
		return nil
	}
	rows := make([]map[string]any, len(contacts))
	for i, c := range contacts {
		rows[i] = map[string]any{"activity_id": activity, "contact_id": uint(c)}
	}
	return db.Table("activity_contacts").Create(rows).Error
}

func one(db *gorm.DB, actor domain.UserID, m models.Activity) (domain.Activity, error) {
	list, err := withContacts(db, actor, []models.Activity{m})
	if err != nil {
		return domain.Activity{}, err
	}
	return list[0], nil
}

// withContacts attaches the actor's live contacts to each activity, with only the values an
// activity shows.
func withContacts(db *gorm.DB, actor domain.UserID, found []models.Activity) ([]domain.Activity, error) {
	out := make([]domain.Activity, len(found))
	if len(found) == 0 {
		return out, nil
	}
	ids := make([]uint, len(found))
	for i, m := range found {
		ids[i] = m.ID
	}
	var joins []struct{ ActivityID, ContactID uint }
	if err := db.Table("activity_contacts").Where("activity_id IN ?", ids).Order("contact_id").Scan(&joins).Error; err != nil {
		return nil, err
	}
	var contacts []models.Contact
	if len(joins) > 0 {
		contactIDs := make([]uint, len(joins))
		for i, j := range joins {
			contactIDs[i] = j.ContactID
		}
		err := db.Select("ID", "Firstname", "Lastname", "Circles").
			Where("user_id = ? AND id IN ?", uint(actor), contactIDs).Find(&contacts).Error
		if err != nil {
			return nil, err
		}
	}
	byID := map[uint]domain.ContactRef{}
	for _, c := range contacts {
		byID[c.ID] = domain.ContactRef{ID: domain.ContactID(c.ID), Firstname: c.Firstname, Lastname: c.Lastname, Circles: c.Circles}
	}
	refs := map[uint][]domain.ContactRef{}
	for _, j := range joins {
		if ref, ok := byID[j.ContactID]; ok {
			refs[j.ActivityID] = append(refs[j.ActivityID], ref)
		}
	}
	for i, m := range found {
		out[i] = toDomain(m, refs[m.ID])
	}
	return out, nil
}

func contactIDs(ids []domain.ContactID) []uint {
	out := make([]uint, len(ids))
	for i, id := range ids {
		out[i] = uint(id)
	}
	return out
}

func toDomain(m models.Activity, contacts []domain.ContactRef) domain.Activity {
	a := domain.Activity{
		ID: domain.ActivityID(m.ID), UserID: domain.UserID(m.UserID), CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
		Title: m.Title, Description: m.Description, Location: m.Location, Date: m.Date, EndDate: m.EndDate, Contacts: contacts,
	}
	if m.DeletedAt.Valid {
		t := m.DeletedAt.Time
		a.DeletedAt = &t
	}
	return a
}
