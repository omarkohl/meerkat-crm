package webhooks

import (
	"context"

	"meerkat/clock"
	"meerkat/reminders/internal/app"
	"meerkat/reminders/internal/domain"
)

type Trigger func(userID uint, event string, data any, clk clock.Clock)

type Events struct{ trigger Trigger }

func New(trigger Trigger) *Events { return &Events{trigger: trigger} }

var _ app.DigestEvents = (*Events)(nil)

func (e *Events) ReminderTriggered(_ context.Context, r domain.DueReminder, clk clock.Clock) {
	go e.trigger(uint(r.UserID), "reminder.triggered", r.Event, clk)
}

func (e *Events) BirthdayOccurred(_ context.Context, user domain.UserID, b domain.Birthday, clk clock.Clock) {
	go e.trigger(uint(user), "birthday.occurred", b.Event, clk)
}
