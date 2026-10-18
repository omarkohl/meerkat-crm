// Package exports is the public entry of the exports module (the CSV and vCard downloads):
// it builds the module from its dependencies and registers its routes. Everything else is
// under internal/ (docs/adr/0012-module-layout.md).
package exports

import (
	"meerkat/config"
	httpadapter "meerkat/exports/internal/adapters/http"
	"meerkat/exports/internal/adapters/persistence"
	"meerkat/exports/internal/app"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Deps are the concrete things the module is built on.
type Deps struct {
	DB     *gorm.DB
	Config *config.Config
}

type Module struct{ uc *app.UseCases }

func New(d Deps) *Module {
	var photoDir string
	if d.Config != nil {
		photoDir = d.Config.ProfilePhotoDir
	}
	repo := persistence.New(d.DB, photoDir)
	return &Module{uc: app.New(repo, repo, persistence.NewAudit(d.DB))}
}

func (m *Module) Register(protected gin.IRoutes) { httpadapter.Register(protected, m.uc) }
