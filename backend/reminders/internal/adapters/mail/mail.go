package mail

import (
	"context"
	"strconv"
	"time"

	"meerkat/clock"
	"meerkat/i18n"
	"meerkat/logger"
	"meerkat/reminders/internal/app"
	"meerkat/reminders/internal/domain"
	"meerkat/services"
)

type Queue func(userID *uint, msg services.EmailMessage, clk clock.Clock) error

type Mailer struct {
	enabled bool
	queue   Queue
}

func New(enabled bool, queue Queue) *Mailer { return &Mailer{enabled: enabled, queue: queue} }

var _ app.DigestMail = (*Mailer)(nil)

func (m *Mailer) Enabled() bool { return m.enabled }

func (m *Mailer) Queue(_ context.Context, d domain.Digest, clk clock.Clock) error {
	lang := d.Recipient.Language
	if lang == "" {
		lang = i18n.DefaultLanguage
	}
	dateFormat := d.Recipient.DateFormat
	if dateFormat == "" {
		dateFormat = "eu"
	}

	reminders := make([]services.ReminderItem, len(d.Reminders))
	for i, r := range d.Reminders {
		name := i18n.T(lang, "email.reminder.unknownContact")
		if r.ContactKnown {
			name = r.ContactName
		}
		reminders[i] = services.ReminderItem{Date: formatDate(r.RemindAt, dateFormat), Message: r.Message, ContactName: name}
	}
	birthdays := make([]services.BirthdayItem, len(d.Birthdays))
	for i, b := range d.Birthdays {
		daysText, badge := badgeFor(lang, b.DaysUntil)
		birthdays[i] = services.BirthdayItem{
			FormattedDate: formatBirthday(b.Birthday, dateFormat), Name: b.Name, DaysText: daysText, BadgeType: badge,
			IsRelationship: b.Type == "relationship", AssociatedContactName: b.AssociatedContactName, RelationshipType: b.RelationshipType,
		}
	}

	html, err := services.RenderReminderEmail(services.ReminderEmailData{
		RemindersTitle: i18n.T(lang, "email.reminder.remindersTitle"),
		BirthdaysTitle: i18n.T(lang, "email.reminder.birthdaysTitle"),
		ContactLabel:   i18n.T(lang, "email.reminder.contactLabel"),
		Footer:         i18n.T(lang, "email.footer"),
		Reminders:      reminders,
		Birthdays:      birthdays,
	})
	if err != nil {
		logger.Error().Err(err).Uint("user_id", uint(d.Recipient.ID)).Msg("Failed to render reminder email template")
		return err
	}
	logger.Debug().Int("reminder_count", len(reminders)).Int("birthday_count", len(birthdays)).Uint("user_id", uint(d.Recipient.ID)).Str("language", lang).Msg("Sending reminder email")
	id := uint(d.Recipient.ID)
	if err := m.queue(&id, services.EmailMessage{
		To: d.Recipient.Email, Subject: i18n.T(lang, "email.reminder.subject"), HTML: html,
	}, clk); err != nil {
		logger.Error().Err(err).Uint("user_id", id).Msg("Failed to queue reminder email")
		return err
	}
	logger.Info().Uint("user_id", id).Msg("Reminder email queued")
	return nil
}

func badgeFor(lang string, days int) (text, badge string) {
	switch days {
	case 0:
		return i18n.T(lang, "email.reminder.today"), "today"
	case 1:
		return i18n.T(lang, "email.reminder.tomorrow"), "tomorrow"
	default:
		return i18n.T(lang, "email.reminder.inDays", map[string]string{"days": strconv.Itoa(days)}), "future"
	}
}

func formatDate(t time.Time, dateFormat string) string {
	switch dateFormat {
	case "us":
		return t.Format("01/02/2006")
	case "iso":
		return t.Format("2006-01-02")
	default:
		return t.Format("02.01.2006")
	}
}

func formatBirthday(birthday, dateFormat string) string {
	if birthday == "" {
		return ""
	}
	if len(birthday) >= 7 && birthday[:2] == "--" {
		month, day := birthday[2:4], birthday[5:7]
		switch dateFormat {
		case "us":
			return month + "/" + day
		case "iso":
			return month + "-" + day
		default:
			return day + "." + month + "."
		}
	}
	if len(birthday) >= 10 && birthday[:2] != "--" {
		year, month, day := birthday[0:4], birthday[5:7], birthday[8:10]
		switch dateFormat {
		case "us":
			return month + "/" + day + "/" + year
		case "iso":
			return year + "-" + month + "-" + day
		default:
			return day + "." + month + "." + year
		}
	}
	return birthday
}
