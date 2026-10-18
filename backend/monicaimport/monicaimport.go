// Package monicaimport is the public entry of the Monica import module: it builds the module
// from its dependencies and registers its HTTP operations. Everything else is under internal/
// (docs/adr/0012-module-layout.md).
package monicaimport

import (
	"meerkat/clock"
	"meerkat/config"
	httpadapter "meerkat/monicaimport/internal/adapters/http"
	"meerkat/monicaimport/internal/adapters/persistence"
	"meerkat/monicaimport/internal/app"

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
	wizard *persistence.Wizard
}

func New(d Deps) *Module {
	cfg := d.Config
	if cfg == nil {
		cfg = &config.Config{}
	}
	wizard := persistence.New(d.DB, cfg)
	return &Module{uc: app.New(wizard), wizard: wizard}
}

// SetClock gives the wizard sessions the application clock (set once at startup, before serving).
func (m *Module) SetClock(c clock.Clock) { m.wizard.SetClock(c) }

func (m *Module) Register(a huma.API, group gin.IRoutes) { httpadapter.Register(a, group, m.uc) }
