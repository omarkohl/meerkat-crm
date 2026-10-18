// Package admin is the public entry of the admin module (backups, the audit log and the
// system status): it builds the module from its dependencies and registers its HTTP routes.
// Everything else is under internal/ (docs/adr/0012-module-layout.md).
package admin

import (
	"context"
	"os"

	httpadapter "meerkat/admin/internal/adapters/http"
	"meerkat/admin/internal/adapters/persistence"
	"meerkat/admin/internal/app"
	"meerkat/admin/internal/domain"
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
	photoDir := cfg.ProfilePhotoDir
	if photoDir == "" {
		photoDir = os.Getenv("PROFILE_PHOTO_DIR")
	}
	settings := domain.SystemSettings{
		Backup: domain.BackupSettings{
			Enabled: cfg.BackupEnabled, Time: cfg.BackupTime, RetentionCount: cfg.BackupRetentionCount, Directory: cfg.BackupDir,
		},
		AuditRetentionDays: cfg.AuditLogRetentionDays,
		PrivateURLsAllowed: cfg.PrivateURLsAllowed(),
		PrivateURLFeatures: cfg.PrivateURLFeatures(),
	}
	return &Module{uc: app.New(
		persistence.NewBackups(d.DB, cfg.BackupDir, photoDir, cfg.BackupRetentionCount),
		persistence.NewAuditLog(d.DB), persistence.NewAudit(d.DB), persistence.NewUsers(d.DB), settings,
	)}
}

// Register adds the routes, with their /admin paths, to admin, the group behind the authentication and the admin
// middleware (which also refuses API tokens: backups hold every user's data).
func (m *Module) Register(a huma.API, admin gin.IRoutes) { httpadapter.Register(a, admin, m.uc) }

// PrivateURLWarning is what the system status reports; main logs it at startup.
func (m *Module) PrivateURLWarning(ctx context.Context) (warn bool, users int64, err error) {
	s, err := m.uc.SystemStatus(ctx)
	return s.PrivateURLWarning, s.UserCount, err
}
