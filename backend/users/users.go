// Package users is the public entry of the users module: it builds the module from its
// dependencies and registers its HTTP operations. Everything else is under internal/
// (docs/adr/0012-module-layout.md).
package users

import (
	"meerkat/config"
	"meerkat/i18n"
	httpadapter "meerkat/users/internal/adapters/http"
	"meerkat/users/internal/adapters/persistence"
	"meerkat/users/internal/app"

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
	return &Module{uc: app.New(persistence.New(d.DB), i18n.SupportedLanguages)}
}

func (m *Module) Register(a huma.API, group gin.IRoutes) {
	httpadapter.Register(a, group, m.uc)
}
