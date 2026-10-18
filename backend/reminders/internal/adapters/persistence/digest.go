package persistence

import (
	"context"
	"time"

	"meerkat/models"
	"meerkat/reminders/internal/app"
	"meerkat/reminders/internal/domain"
	"meerkat/services"

	"gorm.io/gorm"
)

// Digest reads across all users on purpose: the daily job mails everyone. Each reminder's
// contact is looked up with its owner's ID.
type Digest struct{ db *gorm.DB }

func NewDigest(db *gorm.DB) *Digest { return &Digest{db: db} }

var _ app.DigestSource = (*Digest)(nil)

func (d *Digest) DueByMail(ctx context.Context, by time.Time) ([]domain.DueReminder, error) {
	db := d.db.WithContext(ctx)
	var found []models.Reminder
	err := db.Where("by_mail = ? AND remind_at <= ? AND completed = ? AND email_sent = ?", true, by, false, false).Find(&found).Error
	if err != nil {
		return nil, err
	}
	out := make([]domain.DueReminder, len(found))
	for i, m := range found {
		out[i] = domain.DueReminder{Reminder: toDomain(m), Event: m}
		if m.ContactID != nil {
			var c models.Contact
			if err := db.Where("user_id = ?", m.UserID).First(&c, *m.ContactID).Error; err == nil {
				out[i].ContactName, out[i].ContactKnown = c.Firstname+" "+c.Lastname, true
			}
		}
	}
	return out, nil
}

func (d *Digest) UserIDs(ctx context.Context) ([]domain.UserID, error) {
	var ids []uint
	if err := d.db.WithContext(ctx).Model(&models.User{}).Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	out := make([]domain.UserID, len(ids))
	for i, id := range ids {
		out[i] = domain.UserID(id)
	}
	return out, nil
}

func (d *Digest) Recipients(ctx context.Context, ids []domain.UserID) ([]domain.Recipient, error) {
	raw := make([]uint, len(ids))
	for i, id := range ids {
		raw[i] = uint(id)
	}
	var users []models.User
	if err := d.db.WithContext(ctx).Where("id IN ?", raw).Find(&users).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Recipient, len(users))
	for i, u := range users {
		out[i] = domain.Recipient{ID: domain.UserID(u.ID), Email: u.Email, Language: u.Language, DateFormat: u.DateFormat}
	}
	return out, nil
}

func (d *Digest) Birthdays(ctx context.Context, user domain.UserID, now time.Time) ([]domain.Birthday, error) {
	found, err := services.GetUpcomingBirthdays(d.db.WithContext(ctx), uint(user), now)
	out := make([]domain.Birthday, len(found))
	for i, b := range found {
		out[i] = domain.Birthday{
			Type: b.Type, Name: b.Name, Birthday: b.Birthday, RelationshipType: b.RelationshipType, AssociatedContactName: b.AssociatedContactName,
			DaysUntil: services.DaysUntilBirthday(b.Birthday, now), Event: b,
		}
	}
	return out, err
}

func (d *Digest) MarkEmailed(ctx context.Context, user domain.UserID, id domain.ReminderID, at time.Time) error {
	return d.db.WithContext(ctx).Model(&models.Reminder{}).Where("id = ? AND user_id = ?", uint(id), uint(user)).
		Updates(map[string]any{"email_sent": true, "last_sent": at}).Error
}
