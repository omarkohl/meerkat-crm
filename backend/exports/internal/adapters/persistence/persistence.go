// Package persistence implements the ports of the exports module on top of GORM and the
// existing models. It is the only code of the module that imports gorm or models
// (docs/adr/0012-module-layout.md), and the place where every query is scoped to the actor.
package persistence

import (
	"bytes"
	"context"

	"meerkat/carddav"
	"meerkat/exports/internal/app"
	"meerkat/exports/internal/domain"
	"meerkat/models"
	"meerkat/services"

	"github.com/emersion/go-vcard"
	"gorm.io/gorm"
)

type Repository struct {
	db       *gorm.DB
	photoDir string
}

func New(db *gorm.DB, photoDir string) *Repository { return &Repository{db: db, photoDir: photoDir} }

var (
	_ app.Source = (*Repository)(nil)
	_ app.Cards  = (*Repository)(nil)
)

func fetch(what string, err error) error {
	if err == nil {
		return nil
	}
	return &app.FetchError{What: what, Err: err}
}

func (r *Repository) Dataset(ctx context.Context, actor domain.UserID) (domain.Dataset, error) {
	db := r.db.WithContext(ctx)
	var out domain.Dataset

	var user models.User
	if err := db.First(&user, uint(actor)).Error; err != nil {
		return out, fetch("user", err)
	}
	out.CustomFieldNames = user.CustomFieldNames
	if out.CustomFieldNames == nil {
		out.CustomFieldNames = []string{}
	}

	var contacts []models.Contact
	if err := db.Where("user_id = ?", uint(actor)).Preload("Relationships").Order("firstname ASC, lastname ASC").Find(&contacts).Error; err != nil {
		return out, fetch("contacts", err)
	}
	var activities []models.Activity
	if err := db.Where("user_id = ?", uint(actor)).Preload("Contacts").Order("date DESC").Find(&activities).Error; err != nil {
		return out, fetch("activities", err)
	}
	var notes []models.Note
	if err := db.Where("user_id = ?", uint(actor)).Preload("Contact").Order("date DESC").Find(&notes).Error; err != nil {
		return out, fetch("notes", err)
	}
	var reminders []models.Reminder
	if err := db.Where("user_id = ?", uint(actor)).Preload("Contact").Order("remind_at ASC").Find(&reminders).Error; err != nil {
		return out, fetch("reminders", err)
	}

	for _, c := range contacts {
		row := domain.Contact{
			ID: c.ID, Firstname: c.Firstname, Lastname: c.Lastname, Nickname: c.Nickname, Gender: c.Gender, Email: c.Email,
			Phone: c.Phone, Birthday: c.Birthday, Address: c.Address, HowWeMet: c.HowWeMet, FoodPreference: c.FoodPreference,
			WorkInformation: c.WorkInformation, ContactInformation: c.ContactInformation, Circles: c.Circles,
			CustomFields: c.CustomFields, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
		}
		for _, rel := range c.Relationships {
			rr := domain.Relationship{
				ID: rel.ID, Name: rel.Name, Type: rel.Type, Gender: rel.Gender, Birthday: rel.Birthday,
				RelatedContactID: rel.RelatedContactID, CreatedAt: rel.CreatedAt, UpdatedAt: rel.UpdatedAt,
			}
			if rel.RelatedContact != nil {
				rr.RelatedContactName = domain.ContactName(rel.RelatedContact.Firstname, rel.RelatedContact.Lastname)
			}
			row.Relationships = append(row.Relationships, rr)
		}
		out.Contacts = append(out.Contacts, row)
	}
	for _, a := range activities {
		names := make([]string, len(a.Contacts))
		for i, c := range a.Contacts {
			names[i] = domain.ContactName(c.Firstname, c.Lastname)
		}
		out.Activities = append(out.Activities, domain.Activity{
			ID: a.ID, Title: a.Title, Description: a.Description, Location: a.Location, Date: a.Date,
			ContactNames: names, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
		})
	}
	for _, n := range notes {
		row := domain.Note{ID: n.ID, ContactID: n.ContactID, Content: n.Content, Date: n.Date, CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt}
		if n.ContactID != nil {
			row.ContactName = domain.ContactName(n.Contact.Firstname, n.Contact.Lastname)
		}
		out.Notes = append(out.Notes, row)
	}
	for _, m := range reminders {
		row := domain.Reminder{
			ID: m.ID, ContactID: m.ContactID, Message: m.Message, RemindAt: m.RemindAt, Recurrence: m.Recurrence, ByMail: m.ByMail,
			ReoccurFromCompletion: m.ReoccurFromCompletion, Completed: m.Completed, LastSent: m.LastSent,
			CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
		}
		if m.ContactID != nil {
			row.ContactName = domain.ContactName(m.Contact.Firstname, m.Contact.Lastname)
		}
		out.Reminders = append(out.Reminders, row)
	}
	return out, nil
}

// Encode skips a contact that cannot be encoded rather than failing the whole export.
func (r *Repository) Encode(ctx context.Context, actor domain.UserID) (app.Encoded, error) {
	var contacts []models.Contact
	if err := r.db.WithContext(ctx).Where("user_id = ?", uint(actor)).Order("firstname ASC, lastname ASC").Find(&contacts).Error; err != nil {
		return app.Encoded{}, fetch("contacts", err)
	}
	var buf bytes.Buffer
	enc := vcard.NewEncoder(&buf)
	out := app.Encoded{Contacts: len(contacts)}
	for i := range contacts {
		if err := enc.Encode(carddav.ContactToVCard(&contacts[i], r.photoDir)); err != nil {
			out.Skipped = append(out.Skipped, contacts[i].ID)
		}
	}
	out.Body = buf.Bytes()
	return out, nil
}

type Audit struct{ db *gorm.DB }

func NewAudit(db *gorm.DB) *Audit { return &Audit{db: db} }

var _ app.Audit = (*Audit)(nil)

func (a *Audit) Record(ctx context.Context, actor domain.UserID, event, detail string, client domain.Client) {
	services.WriteUserAudit(a.db.WithContext(ctx), uint(actor), event, detail, client.IP, client.UserAgent)
}
