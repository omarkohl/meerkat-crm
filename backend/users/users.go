// Package users is the public entry of the users module: it builds the module from its
// dependencies and registers its HTTP operations. Everything else is under internal/
// (docs/adr/0012-module-layout.md).
package users

import (
	"meerkat/config"
	"meerkat/i18n"
	"meerkat/services"
	httpadapter "meerkat/users/internal/adapters/http"
	"meerkat/users/internal/adapters/lockout"
	"meerkat/users/internal/adapters/oidc"
	"meerkat/users/internal/adapters/passwords"
	"meerkat/users/internal/adapters/persistence"
	"meerkat/users/internal/adapters/resettokens"
	"meerkat/users/internal/adapters/sessiontokens"
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
	// OIDC is the single sign-on provider, nil when it is not configured.
	OIDC *services.OIDCProvider
}

type Module struct {
	uc   *app.UseCases
	cfg  config.Config
	oidc bool
}

func New(d Deps) *Module {
	var cfg config.Config
	if d.Config != nil {
		cfg = *d.Config
	}
	var identity app.Identity
	if d.OIDC != nil {
		identity = oidc.New(d.OIDC, &cfg)
	}
	uc := app.New(persistence.New(d.DB), i18n.SupportedLanguages,
		persistence.NewSessions(d.DB, &cfg), persistence.NewAccounts(d.DB, &cfg), passwords.Bcrypt{}, persistence.NewAudit(d.DB),
		persistence.NewTwoFactor(d.DB, &cfg), lockout.Limiter{}, resettokens.Tokens{}, persistence.NewMail(d.DB, &cfg),
		lockout.Identifiers{}, sessiontokens.New(&cfg), identity, cfg.RegistrationDisabled)
	return &Module{uc: uc, cfg: cfg, oidc: d.OIDC != nil}
}

func (m *Module) Register(a huma.API, public, protected gin.IRoutes) {
	httpadapter.Register(a, public, protected, m.uc, httpadapter.Cookie{
		Domain: m.cfg.CookieDomain, Secure: m.cfg.CookieSecure, MaxAge: m.cfg.JWTExpiryHours * 3600, Issuable: m.cfg.JWTSecretKey != "",
	}, httpadapter.OIDCSettings{Enabled: m.cfg.OIDC.Enabled, ProviderURL: m.cfg.OIDC.ProviderURL, Routes: m.cfg.OIDC.Enabled && m.oidc})
}
