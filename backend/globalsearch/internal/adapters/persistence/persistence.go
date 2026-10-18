// Package persistence finds the actor's contacts, notes and activities with the FTS5 indexes
// (migrations 29 and 34), and with LIKE where the index is the wrong tool (docs/adr/0012-module-layout.md).
package persistence

import (
	"context"
	"sort"
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
		for _, col := range []string{"middle_name", "organization", "department", "job_title", "role", "how_we_met", "food_preference", "work_information", "contact_information", "note", "address"} {
			cond += " OR LOWER(contacts." + col + ") LIKE ? ESCAPE '\\'"
			args = append(args, like)
		}
		cond += " OR EXISTS (SELECT 1 FROM json_each(CASE WHEN json_valid(contacts.addresses) AND json_type(contacts.addresses) = 'array' THEN contacts.addresses ELSE '[]' END) AS j WHERE LOWER(COALESCE(json_extract(j.value, '$.street'), '') || ' ' || COALESCE(json_extract(j.value, '$.city'), '') || ' ' || COALESCE(json_extract(j.value, '$.region'), '') || ' ' || COALESCE(json_extract(j.value, '$.postal'), '') || ' ' || COALESCE(json_extract(j.value, '$.country'), '')) LIKE ? ESCAPE '\\')" +
			" OR EXISTS (SELECT 1 FROM json_each(CASE WHEN json_valid(contacts.circles) AND json_type(contacts.circles) = 'array' THEN contacts.circles ELSE '[]' END) AS j WHERE LOWER(j.value) LIKE ? ESCAPE '\\')" +
			" OR EXISTS (SELECT 1 FROM json_each(CASE WHEN json_valid(contacts.custom_fields) AND json_type(contacts.custom_fields) = 'object' THEN contacts.custom_fields ELSE '{}' END) AS j WHERE LOWER(j.value) LIKE ? ESCAPE '\\')"
		args = append(args, like, like, like)
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
		page.Items = append(page.Items, toDomain(c))
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
	people, err := r.participants(db, actor, found)
	if err != nil {
		return app.Page[domain.Activity]{}, err
	}
	page := app.Page[domain.Activity]{Total: int(total), Substring: !indexed}
	for _, a := range found {
		page.Items = append(page.Items, domain.Activity{ID: a.ID, Title: a.Title, Description: a.Description, Location: a.Location, Date: a.Date, Contacts: people[a.ID]})
	}
	return page, nil
}

func toDomain(c models.Contact) domain.Contact {
	hit := domain.Contact{ID: c.ID, Firstname: c.Firstname, Lastname: c.Lastname, Nickname: c.Nickname}
	hit.Emails = append(hit.Emails, c.Email)
	for _, e := range c.Emails {
		hit.Emails = append(hit.Emails, e.Value)
	}
	hit.Phones = append(hit.Phones, c.Phone)
	for _, p := range c.Phones {
		hit.Phones = append(hit.Phones, p.Value)
	}
	add := func(key, value string) {
		if value != "" {
			hit.Others = append(hit.Others, domain.Field{Key: key, Value: value})
		}
	}
	add("organization", c.Organization)
	add("job_title", c.JobTitle)
	add("department", c.Department)
	add("role", c.Role)
	add("middle_name", c.MiddleName)
	for _, a := range c.Addresses {
		add("address", models.FormatAddress(a))
	}
	add("address", c.Address)
	for _, circle := range c.Circles {
		add("circles", circle)
	}
	add("how_we_met", c.HowWeMet)
	add("food_preference", c.FoodPreference)
	add("work_information", c.WorkInformation)
	add("contact_information", c.ContactInformation)
	add("note", c.Note)
	names := make([]string, 0, len(c.CustomFields))
	for name := range c.CustomFields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if v := c.CustomFields[name]; v != "" {
			hit.Others = append(hit.Others, domain.Field{Key: "custom_field", Name: name, Value: v})
		}
	}
	return hit
}

// participants names, per activity, the actor's contacts that are not in the trash.
func (r *Repository) participants(db *gorm.DB, actor domain.UserID, activities []models.Activity) (map[uint][]string, error) {
	people := map[uint][]string{}
	if len(activities) == 0 {
		return people, nil
	}
	ids := make([]uint, len(activities))
	for i, a := range activities {
		ids[i] = a.ID
	}
	var rows []struct {
		ActivityID uint
		Firstname  string
		Lastname   string
		Nickname   string
	}
	err := db.Table("activity_contacts").
		Select("activity_contacts.activity_id, contacts.firstname, contacts.lastname, contacts.nickname").
		Joins("JOIN contacts ON contacts.id = activity_contacts.contact_id AND contacts.user_id = ? AND contacts.deleted_at IS NULL", uint(actor)).
		Where("activity_contacts.activity_id IN ?", ids).
		Order("contacts.firstname, contacts.lastname, contacts.id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		people[row.ActivityID] = append(people[row.ActivityID], domain.Contact{Firstname: row.Firstname, Lastname: row.Lastname, Nickname: row.Nickname}.DisplayName())
	}
	return people, nil
}
