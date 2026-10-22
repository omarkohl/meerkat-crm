// Package images is the public entry of the images module: pictures of contacts, activities
// and notes. It builds the module from its dependencies and registers its HTTP operations.
// Everything else is under internal/ (docs/adr/0012-module-layout.md).
package images

import (
	"context"
	"time"

	"meerkat/config"
	"meerkat/images/internal/adapters/files"
	httpadapter "meerkat/images/internal/adapters/http"
	"meerkat/images/internal/adapters/persistence"
	"meerkat/images/internal/app"

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
	return &Module{uc: app.New(persistence.New(d.DB), files.New(dir))}
}

func (m *Module) Register(a huma.API, group gin.IRoutes) {
	httpadapter.Register(a, group, m.uc)
}

// Sweep deletes pending uploads that nobody claimed and files no row refers to. It returns how many it removed.
func (m *Module) Sweep(ctx context.Context, now time.Time) (pending, orphans int, err error) {
	res, err := m.uc.Sweep(ctx, now)
	return res.Pending, res.Orphans, err
}
