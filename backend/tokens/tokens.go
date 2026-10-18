// Package tokens is the public entry of the tokens module (API tokens and the calendar feed):
// it builds the module from its dependencies and registers its HTTP routes. Everything else
// is under internal/ (docs/adr/0012-module-layout.md).
package tokens

import (
	httpadapter "meerkat/tokens/internal/adapters/http"
	"meerkat/tokens/internal/adapters/persistence"
	"meerkat/tokens/internal/app"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Deps are the concrete things the module is built on. DB may be nil when only the
// operations' descriptions are needed (cmd/openapi): the handlers are then never run.
type Deps struct{ DB *gorm.DB }

type Module struct{ uc *app.UseCases }

func New(d Deps) *Module {
	return &Module{uc: app.New(persistence.NewTokens(d.DB), persistence.NewFeeds(d.DB), persistence.NewAudit(d.DB))}
}

func (m *Module) Register(a huma.API, public, protected gin.IRoutes) {
	httpadapter.Register(a, public, protected, m.uc)
}
