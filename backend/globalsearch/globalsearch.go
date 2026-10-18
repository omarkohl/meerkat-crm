// Package globalsearch is the public entry of the global search module: one search over
// contacts, notes and activities. Everything else is under internal/
// (docs/adr/0012-module-layout.md).
package globalsearch

import (
	httpadapter "meerkat/globalsearch/internal/adapters/http"
	"meerkat/globalsearch/internal/adapters/persistence"
	"meerkat/globalsearch/internal/app"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Deps are the concrete things the module is built on. DB may be nil when only the
// operations' descriptions are needed (cmd/openapi): the handlers are then never run.
type Deps struct{ DB *gorm.DB }

type Module struct{ uc *app.UseCases }

func New(d Deps) *Module { return &Module{uc: app.New(persistence.New(d.DB))} }

func (m *Module) Register(a huma.API, group gin.IRoutes) {
	httpadapter.Register(a, group, m.uc)
}
