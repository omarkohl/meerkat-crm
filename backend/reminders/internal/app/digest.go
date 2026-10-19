package app

import (
	"context"
	"fmt"
	"sort"
	"time"

	"meerkat/clock"
	"meerkat/logger"
	"meerkat/reminders/internal/domain"
)

type DigestSource interface {
	DueByMail(ctx context.Context, by time.Time) ([]domain.DueReminder, error)
	UserIDs(ctx context.Context) ([]domain.UserID, error)
	Recipients(ctx context.Context, ids []domain.UserID) ([]domain.Recipient, error)
	Birthdays(ctx context.Context, user domain.UserID, now time.Time) ([]domain.Birthday, error)
	MarkBirthdaysMailed(ctx context.Context, user domain.UserID, day string) error
	MarkEmailed(ctx context.Context, user domain.UserID, id domain.ReminderID, at time.Time) error
}

type DigestMail interface {
	Enabled() bool
	Queue(ctx context.Context, d domain.Digest, clk clock.Clock) error
}

type DigestEvents interface {
	ReminderTriggered(ctx context.Context, r domain.DueReminder, clk clock.Clock)
	BirthdayOccurred(ctx context.Context, user domain.UserID, b domain.Birthday, clk clock.Clock)
}

type Digest struct {
	source DigestSource
	mail   DigestMail
	events DigestEvents
	loc    *time.Location
}

func NewDigest(source DigestSource, mail DigestMail, events DigestEvents, reminderLocation *time.Location) *Digest {
	return &Digest{source: source, mail: mail, events: events, loc: reminderLocation}
}

// With no e-mail channel nothing is mailed and the reminders stay unmarked, to go out once one is set.
func (d *Digest) Run(ctx context.Context, clk clock.Clock) error {
	logger.Info().Msg("Sending reminders...")
	now := clk.Now().In(d.loc)

	due, err := d.source.DueByMail(ctx, domain.EndOfDay(now))
	if err != nil {
		return fmt.Errorf("failed to fetch reminders: %w", err)
	}
	dueByUser := make(map[domain.UserID][]domain.DueReminder)
	for _, r := range due {
		dueByUser[r.UserID] = append(dueByUser[r.UserID], r)
	}
	today := now.Format("2006-01-02")
	all, err := d.source.UserIDs(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to fetch all users for birthday check, continuing with reminders only")
		for id := range dueByUser {
			all = append(all, id)
		}
	}
	recipients, err := d.source.Recipients(ctx, all)
	if err != nil {
		return fmt.Errorf("failed to fetch users: %w", err)
	}
	byID := make(map[domain.UserID]domain.Recipient, len(recipients))
	for _, r := range recipients {
		byID[r.ID] = r
	}
	ids := d.withBirthdayToday(ctx, dueByUser, all, byID, now, today)
	if len(ids) == 0 {
		logger.Info().Msg("No reminders or birthdays to send for today")
		return nil
	}

	failed := 0
	for _, id := range ids {
		recipient, ok := byID[id]
		if !ok {
			logger.Warn().Uint("user_id", uint(id)).Msg("Skipping user - not found")
			continue
		}
		var birthdays []domain.Birthday
		if recipient.BirthdayMailedOn != today {
			var err error
			birthdays, err = d.source.Birthdays(ctx, id, now)
			if err != nil {
				logger.Warn().Err(err).Uint("user_id", uint(id)).Msg("Failed to fetch birthdays, continuing without them")
			}
		}
		if !d.mailUser(ctx, clk, domain.Digest{Recipient: recipient, Reminders: dueByUser[id], Birthdays: birthdays}, today) {
			failed++
		}
		d.publish(ctx, clk, id, dueByUser[id], birthdays)
	}
	if failed > 0 {
		logger.Warn().Int("failed_users", failed).Int("total_users", len(ids)).Msg("Some emails failed to send")
	}
	return nil
}

func (d *Digest) withBirthdayToday(ctx context.Context, dueByUser map[domain.UserID][]domain.DueReminder, all []domain.UserID, byID map[domain.UserID]domain.Recipient, now time.Time, today string) []domain.UserID {
	set := make(map[domain.UserID]bool, len(dueByUser))
	for id := range dueByUser {
		set[id] = true
	}
	for _, id := range all {
		if set[id] || byID[id].BirthdayMailedOn == today {
			continue
		}
		birthdays, err := d.source.Birthdays(ctx, id, now)
		if err != nil {
			logger.Warn().Err(err).Uint("user_id", uint(id)).Msg("Failed to fetch birthdays for user, continuing")
			continue
		}
		if len(birthdays) > 0 && birthdays[0].Today() {
			set[id] = true
		}
	}
	ids := make([]domain.UserID, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (d *Digest) mailUser(ctx context.Context, clk clock.Clock, digest domain.Digest, today string) bool {
	id := uint(digest.Recipient.ID)
	if !d.mail.Enabled() {
		logger.Info().Int("reminder_count", len(digest.Reminders)).Uint("user_id", id).Msg("Email sending disabled (no channel configured), skipping reminder mutations to preserve them")
		return true
	}
	queued := false
	if digest.Recipient.Email == "" {
		logger.Warn().Uint("user_id", id).Msg("Skipping reminder email because user email is missing")
	} else if err := d.mail.Queue(ctx, digest, clk); err != nil {
		logger.Error().Err(err).Uint("user_id", id).Msg("Error sending daily email, skipping reminder mutations for this user")
		return false
	} else {
		queued = true
	}
	for _, b := range digest.Birthdays {
		if queued && b.Today() {
			if err := d.source.MarkBirthdaysMailed(ctx, digest.Recipient.ID, today); err != nil {
				logger.Error().Err(err).Uint("user_id", id).Msg("Failed to remember that the birthday mail was sent")
			}
			break
		}
	}
	for _, r := range digest.Reminders {
		if err := d.source.MarkEmailed(ctx, r.UserID, r.ID, clk.Now()); err != nil {
			logger.Error().Err(err).Uint("reminder_id", uint(r.ID)).Msg("Failed to update reminder after sending email")
		} else {
			logger.Info().Uint("reminder_id", uint(r.ID)).Msg("Marked reminder as email_sent")
		}
	}
	return true
}

func (d *Digest) publish(ctx context.Context, clk clock.Clock, user domain.UserID, due []domain.DueReminder, birthdays []domain.Birthday) {
	for _, r := range due {
		d.events.ReminderTriggered(ctx, r, clk)
	}
	for _, b := range birthdays {
		if b.Today() {
			d.events.BirthdayOccurred(ctx, user, b, clk)
		}
	}
}
