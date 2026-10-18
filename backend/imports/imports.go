// Package imports is the public entry of the imports module (the CSV and vCard import wizard):
// it builds the module from its dependencies and registers its HTTP operations. Everything
// else is under internal/ (docs/adr/0012-module-layout.md). The Monica import is separate.
package imports

import (
	"meerkat/clock"
	"meerkat/config"
	httpadapter "meerkat/imports/internal/adapters/http"
	"meerkat/imports/internal/adapters/persistence"
	"meerkat/imports/internal/app"

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
