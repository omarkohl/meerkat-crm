// Package webhooks is the public entry of the webhooks module (managing a user's webhooks):
// it builds the module from its dependencies and registers its HTTP operations. Everything
// else is under internal/ (docs/adr/0012-module-layout.md). Delivering events stays in
// services, which every module's events share.
package webhooks

import (
	"meerkat/config"
	httpadapter "meerkat/webhooks/internal/adapters/http"
	"meerkat/webhooks/internal/adapters/persistence"
	"meerkat/webhooks/internal/app"

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
	repo := persistence.New(d.DB, cfg)
	return &Module{uc: app.New(repo, repo)}
}

func (m *Module) Register(a huma.API, group gin.IRoutes) {
	httpadapter.Register(a, group, m.uc)
}
