// Package calendars is the public entry of the calendars module (calendar subscriptions): it
// builds the module from its dependencies and registers its HTTP operations. Everything else is
// under internal/ (docs/adr/0012-module-layout.md).
package calendars

import (
	httpadapter "meerkat/calendars/internal/adapters/http"
	"meerkat/calendars/internal/adapters/persistence"
	"meerkat/calendars/internal/app"
	"meerkat/config"

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
	return &Module{uc: app.New(persistence.New(d.DB, cfg), persistence.NewSecrets(cfg.JWTSecretKey))}
}

func (m *Module) Register(a huma.API, group gin.IRoutes) { httpadapter.Register(a, group, m.uc) }
