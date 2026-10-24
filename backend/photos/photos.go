// Package photos is the public entry of the photos module: it builds the module from its
// dependencies and registers its HTTP operations. Everything else is under internal/
// (docs/adr/0012-module-layout.md).
package photos

import (
	"meerkat/config"
	"meerkat/photos/internal/adapters/files"
	httpadapter "meerkat/photos/internal/adapters/http"
	"meerkat/photos/internal/adapters/persistence"
	"meerkat/photos/internal/app"

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
	dir := ""
	if d.Config != nil {
		dir = d.Config.ProfilePhotoDir
	}
	repo := persistence.New(d.DB, dir)
	return &Module{uc: app.New(repo, files.New(dir))}
}

func (m *Module) Register(a huma.API, group gin.IRoutes) {
	httpadapter.Register(a, group, m.uc)
}
