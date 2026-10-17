// Package webhooks implements the trash module's event port with the webhook service.
package webhooks

import (
	"context"
	"time"

	"meerkat/api"
	"meerkat/clock"
	"meerkat/trash/internal/app"
	"meerkat/trash/internal/domain"
)

// Trigger sends the webhook deliveries subscribed to an event (services.TriggerWebhooks,
// bound to a database and configuration by the module's entry package).
type Trigger func(userID uint, event string, data any, clk clock.Clock)

type Publisher struct{ trigger Trigger }

func New(trigger Trigger) *Publisher { return &Publisher{trigger: trigger} }

var _ app.Events = (*Publisher)(nil)

func (p *Publisher) ContactRestored(ctx context.Context, actor domain.UserID, event any, _ time.Time) {
	go p.trigger(uint(actor), "contact.restored", event, api.Clock(ctx))
}
