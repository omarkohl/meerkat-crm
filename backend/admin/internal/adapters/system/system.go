// Package system implements the admin module's ports that are neither storage nor HTTP: the
// reminder job and the in-memory counter of failed second-factor codes.
package system

import (
	"context"

	"meerkat/admin/internal/app"
	"meerkat/admin/internal/domain"
	"meerkat/clock"
	"meerkat/middleware"
)

type Reminders struct{ send func(clock.Clock) error }

func NewReminders(send func(clock.Clock) error) *Reminders { return &Reminders{send: send} }

var _ app.Reminders = (*Reminders)(nil)

func (r *Reminders) Send(_ context.Context, clk clock.Clock) error { return r.send(clk) }

type Lockouts struct{}

var _ app.Lockouts = Lockouts{}

func (Lockouts) ClearSecondFactor(_ context.Context, id domain.UserID) {
	middleware.GetAccountRateLimiter().RecordSuccessfulLogin(middleware.SecondFactorKey(uint(id)))
}
