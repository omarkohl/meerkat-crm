// Package reminders is the public entry of the reminders module: it builds the module from its
// dependencies and registers its HTTP operations. Everything else is under internal/
// (docs/adr/0012-module-layout.md).
package reminders

import (
	"context"

	"meerkat/clock"
	"meerkat/config"
	httpadapter "meerkat/reminders/internal/adapters/http"
	"meerkat/reminders/internal/adapters/mail"
	"meerkat/reminders/internal/adapters/persistence"
	"meerkat/reminders/internal/adapters/webhooks"
	"meerkat/reminders/internal/app"
	"meerkat/services"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Deps are the concrete things the module is built on. DB and Config may be nil when only
// the operations' descriptions are needed (cmd/openapi): the handlers are then never run.
type Deps struct {
	DB     *gorm.DB
	Config *config.Config
}

type Module struct {
	uc     *app.UseCases
	digest *app.Digest
}

func New(d Deps) *Module {
	var cfg config.Config
	if d.Config != nil {
		cfg = *d.Config
	}
	repo := persistence.New(d.DB)
	trigger := func(userID uint, event string, data any, clk clock.Clock) {
		services.TriggerWebhooks(d.DB, cfg, userID, event, data, clk)
	}
	digest := app.NewDigest(persistence.NewDigest(d.DB), mail.New(cfg.EmailEnabled(), func(userID *uint, msg services.EmailMessage, clk clock.Clock) error {
		return services.QueueEmail(d.DB, cfg, userID, msg, clk)
	}), webhooks.New(trigger), cfg.GetReminderLocation())
	return &Module{uc: app.New(repo, repo), digest: digest}
}

func (m *Module) SendDailyDigest(clk clock.Clock) error {
	return m.digest.Run(context.Background(), clk)
}

// Register adds the module's operations to a. group is the Gin group that carries the
// authentication middleware.
func (m *Module) Register(a huma.API, group gin.IRoutes) {
	httpadapter.Register(a, group, m.uc)
}
