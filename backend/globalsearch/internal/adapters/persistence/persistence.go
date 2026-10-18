// Package persistence finds the actor's contacts, notes and activities with the FTS5 indexes
// (migration 29), and with LIKE where the index is the wrong tool (docs/adr/0012-module-layout.md).
package persistence

import (
	"context"
	"strings"
	"unicode"

	"meerkat/globalsearch/internal/app"
	"meerkat/globalsearch/internal/domain"
	"meerkat/models"
	"meerkat/search"

	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

// likePattern escapes what LIKE treats as a wildcard.
func likePattern(term string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(strings.ToLower(term)) + "%"
}

// phoneLike is a term of digits and phone punctuation only.
func phoneLike(term string) bool {
	digit := false
	for _, r := range term {
		switch {
		case unicode.IsDigit(r):
			digit = true
		case strings.ContainsRune(" +()./-", r):
		default:
			return false
		}
	}
	return digit
}

func digitsOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// withoutPunctuation is a SQL expression that drops the characters a phone number is
// written with, so that digits match across them.
func withoutPunctuation(col string) string {
	e := col
	for _, c := range []string{" ", "-", "(", ")", ".", "/", "+"} {
		e = "replace(" + e + ", '" + c + "', '')"
	}
	return e
}

func (r *Repository) Contacts(ctx context.Context, actor domain.UserID, term string, limit, offset int) (app.Page[domain.Contact], error) {
	db := r.db.WithContext(ctx)
	base := db.Model(&models.Contact{}).Where("contacts.user_id = ? AND contacts.deleted_at IS NULL", uint(actor))
	match, indexed := search.MatchQuery(term)
	indexed = indexed && search.Available(db, "contacts_fts")
	var order []any
	if indexed {
		base = base.
			Joins("JOIN "+search.Hits("contacts_fts")+" AS hits ON hits.hit_id = contacts.id", match).
			Joins("LEFT JOIN (SELECT rowid AS name_id FROM contacts_fts WHERE contacts_fts MATCH ?) AS names ON names.name_id = contacts.id", search.ContactNames(match))
		order = []any{"names.name_id IS NULL", "hits.hit_rank", "contacts.id"}
	} else {
		like := likePattern(term)
		name := "LOWER(TRIM(REPLACE(contacts.firstname || ' ' || contacts.nickname || ' ' || contacts.lastname, '  ', ' '))) LIKE ? ESCAPE '\\'"
		values := "CASE WHEN json_valid(contacts.%s) THEN contacts.%s ELSE '[]' END"
		emails := "json_each(" + strings.ReplaceAll(values, "%s", "emails") + ") AS j"
		phones := "json_each(" + strings.ReplaceAll(values, "%s", "phones") + ") AS j"
		cond := "(" + name + ") OR LOWER(contacts.email) LIKE ? ESCAPE '\\' OR LOWER(contacts.phone) LIKE ? ESCAPE '\\'" +
			" OR EXISTS (SELECT 1 FROM " + emails + " WHERE LOWER(json_extract(j.value, '$.value')) LIKE ? ESCAPE '\\')" +
			" OR EXISTS (SELECT 1 FROM " + phones + " WHERE LOWER(json_extract(j.value, '$.value')) LIKE ? ESCAPE '\\')"
		args := []any{like, like, like, like, like}
		if phoneLike(term) {
			digits := "%" + digitsOf(term) + "%"
			cond += " OR " + withoutPunctuation("contacts.phone") + " LIKE ?" +
				" OR EXISTS (SELECT 1 FROM " + phones + " WHERE " + withoutPunctuation("json_extract(j.value, '$.value')") + " LIKE ?)"
			args = append(args, digits, digits)
		}
		base = base.Where(cond, args...)
		order = []any{gorm.Expr("CASE WHEN "+name+" THEN 0 ELSE 1 END", like), "contacts.id"}
	}
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return app.Page[domain.Contact]{}, err
	}
	q := base.Session(&gorm.Session{})
	for _, o := range order {
		q = q.Order(o)
	}
	var found []models.Contact
	if err := q.Select("contacts.*").Limit(limit).Offset(offset).Find(&found).Error; err != nil {
		return app.Page[domain.Contact]{}, err
	}
	page := app.Page[domain.Contact]{Total: int(total), Substring: !indexed}
	for _, c := range found {
		hit := domain.Contact{ID: c.ID, Firstname: c.Firstname, Lastname: c.Lastname, Nickname: c.Nickname}
		hit.Emails = append(hit.Emails, c.Email)
		for _, e := range c.Emails {
			hit.Emails = append(hit.Emails, e.Value)
		}
		hit.Phones = append(hit.Phones, c.Phone)
		for _, p := range c.Phones {
			hit.Phones = append(hit.Phones, p.Value)
		}
		page.Items = append(page.Items, hit)
	}
	return page, nil
}

func (r *Repository) Notes(ctx context.Context, actor domain.UserID, term string, limit, offset int) (app.Page[domain.Note], error) {
	db := r.db.WithContext(ctx)
	base := db.Model(&models.Note{}).Where(
		"notes.user_id = ? AND notes.deleted_at IS NULL AND (notes.contact_id IS NULL OR notes.contact_id IN (SELECT id FROM contacts WHERE user_id = ? AND deleted_at IS NULL))",
		uint(actor), uint(actor))
	match, indexed := search.MatchQuery(term)
	indexed = indexed && search.Available(db, "notes_fts")
	if indexed {
		base = base.Joins("JOIN "+search.Hits("notes_fts")+" AS hits ON hits.hit_id = notes.id", match)
	} else {
		base = base.Where("LOWER(notes.content) LIKE ? ESCAPE '\\'", likePattern(term))
	}
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return app.Page[domain.Note]{}, err
	}
	q := base.Session(&gorm.Session{})
	if indexed {
		q = q.Order("hits.hit_rank")
	}
	var found []models.Note
	if err := q.Order("notes.date DESC, notes.id DESC").Select("notes.*").Limit(limit).Offset(offset).Find(&found).Error; err != nil {
		return app.Page[domain.Note]{}, err
	}
	names, err := r.contactNames(db, actor, found)
	if err != nil {
		return app.Page[domain.Note]{}, err
	}
	page := app.Page[domain.Note]{Total: int(total), Substring: !indexed}
	for _, n := range found {
		note := domain.Note{ID: n.ID, ContactID: n.ContactID, Date: n.Date, Content: n.Content}
		if n.ContactID != nil {
			note.ContactName = names[*n.ContactID]
		}
		page.Items = append(page.Items, note)
	}
	return page, nil
}

func (r *Repository) contactNames(db *gorm.DB, actor domain.UserID, notes []models.Note) (map[uint]string, error) {
	var ids []uint
	for _, n := range notes {
		if n.ContactID != nil {
			ids = append(ids, *n.ContactID)
		}
	}
	names := map[uint]string{}
	if len(ids) == 0 {
		return names, nil
	}
	var contacts []models.Contact
	if err := db.Where("user_id = ? AND id IN ?", uint(actor), ids).Select("id", "firstname", "lastname", "nickname").Find(&contacts).Error; err != nil {
		return nil, err
	}
	for _, c := range contacts {
		names[c.ID] = domain.Contact{Firstname: c.Firstname, Lastname: c.Lastname, Nickname: c.Nickname}.DisplayName()
	}
	return names, nil
}

func (r *Repository) Activities(ctx context.Context, actor domain.UserID, term string, limit, offset int) (app.Page[domain.Activity], error) {
	db := r.db.WithContext(ctx)
	base := db.Model(&models.Activity{}).Where("activities.user_id = ? AND activities.deleted_at IS NULL", uint(actor))
	match, indexed := search.MatchQuery(term)
	indexed = indexed && search.Available(db, "activities_fts")
	if indexed {
		base = base.Joins("JOIN "+search.Hits("activities_fts")+" AS hits ON hits.hit_id = activities.id", match)
	} else {
		like := likePattern(term)
		base = base.Where("LOWER(activities.title) LIKE ? ESCAPE '\\' OR LOWER(activities.description) LIKE ? ESCAPE '\\' OR LOWER(activities.location) LIKE ? ESCAPE '\\'", like, like, like)
	}
	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return app.Page[domain.Activity]{}, err
	}
	q := base.Session(&gorm.Session{})
	if indexed {
		q = q.Order("hits.hit_rank")
	}
	var found []models.Activity
	if err := q.Order("activities.date DESC, activities.id DESC").Select("activities.*").Limit(limit).Offset(offset).Find(&found).Error; err != nil {
		return app.Page[domain.Activity]{}, err
	}
	page := app.Page[domain.Activity]{Total: int(total), Substring: !indexed}
	for _, a := range found {
		page.Items = append(page.Items, domain.Activity{ID: a.ID, Title: a.Title, Description: a.Description, Location: a.Location, Date: a.Date})
	}
	return page, nil
}
