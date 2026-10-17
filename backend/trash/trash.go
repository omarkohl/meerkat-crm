// Package trash is the public entry of the trash module: it builds the module from its
// dependencies and registers its HTTP operations. Everything else is under internal/
// (docs/adr/0012-module-layout.md).
package trash

import (
	"os"

	"meerkat/clock"
	"meerkat/config"
	"meerkat/services"
	httpadapter "meerkat/trash/internal/adapters/http"
	"meerkat/trash/internal/adapters/persistence"
	"meerkat/trash/internal/adapters/webhooks"
	"meerkat/trash/internal/app"

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

type Module struct{ uc *app.UseCases }

func New(d Deps) *Module {
	var cfg config.Config
	if d.Config != nil {
		cfg = *d.Config
	}
	photoDir := cfg.ProfilePhotoDir
	if photoDir == "" {
		photoDir = os.Getenv("PROFILE_PHOTO_DIR")
	}
	trigger := func(userID uint, event string, data any, clk clock.Clock) {
		services.TriggerWebhooks(d.DB, cfg, userID, event, data, clk)
	}
	return &Module{uc: app.New(persistence.New(d.DB, photoDir), webhooks.New(trigger), cfg.TrashRetentionDays)}
}

func (m *Module) Register(a huma.API, group gin.IRoutes) {
	httpadapter.Register(a, group, m.uc)
}
