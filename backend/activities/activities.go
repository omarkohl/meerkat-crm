// Package activities is the public entry of the activities module: it builds the module from its
// dependencies and registers its HTTP operations. Everything else is under internal/
// (docs/adr/0012-module-layout.md).
package activities

import (
	httpadapter "meerkat/activities/internal/adapters/http"
	"meerkat/activities/internal/adapters/persistence"
	"meerkat/activities/internal/adapters/webhooks"
	"meerkat/activities/internal/app"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/photofile"
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

type Module struct{ uc *app.UseCases }

func New(d Deps) *Module {
	repo := persistence.New(d.DB)
	trigger := func(userID uint, event string, data any, clk clock.Clock) {
		services.TriggerWebhooks(d.DB, *d.Config, userID, event, data, clk)
	}
	dir := ""
	if d.Config != nil {
		dir = d.Config.ProfilePhotoDir
	}
	return &Module{uc: app.New(repo, repo, webhooks.New(trigger), imageFiles{dir: dir})}
}

// Register adds the module's operations to a. group is the Gin group that carries the
// authentication middleware.
func (m *Module) Register(a huma.API, group gin.IRoutes) {
	httpadapter.Register(a, group, m.uc)
}

type imageFiles struct{ dir string }

func (f imageFiles) Remove(names ...string) { photofile.Remove(f.dir, names...) }
