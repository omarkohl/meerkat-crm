package routes

import (
	"meerkat/activities"
	"meerkat/admin"
	"meerkat/api"
	"meerkat/calendars"
	"meerkat/carddav"
	"meerkat/config"
	"meerkat/contacts"
	"meerkat/contactsync"
	"meerkat/controllers"
	"meerkat/exports"
	"meerkat/imports"
	"meerkat/metrics"
	"meerkat/middleware"
	"meerkat/monicaimport"
	"meerkat/notes"
	"meerkat/photos"
	"meerkat/relationships"
	"meerkat/reminders"
	"meerkat/services"
	"meerkat/tokens"
	"meerkat/trash"
	"meerkat/users"
	"meerkat/webhooks"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Modules are the converted modules (docs/adr/0012-module-layout.md), built by main.go.
type Modules struct {
	Admin         *admin.Module
	Contacts      *contacts.Module
	Notes         *notes.Module
	Activities    *activities.Module
	Reminders     *reminders.Module
	Relationships *relationships.Module
	Photos        *photos.Module
	Trash         *trash.Module
	Tokens        *tokens.Module
	Webhooks      *webhooks.Module
	Exports       *exports.Module
	Imports       *imports.Module
	MonicaImport  *monicaimport.Module
	Calendars     *calendars.Module
	ContactSync   *contactsync.Module
	Users         *users.Module
}

// NewModules builds every converted module on db. db may be nil when only the operations'
// descriptions are needed.
func NewModules(db *gorm.DB, cfg *config.Config, oidcProvider *services.OIDCProvider) Modules {
	reminderModule := reminders.New(reminders.Deps{DB: db, Config: cfg})
	return Modules{
		Admin:         admin.New(admin.Deps{DB: db, Config: cfg, SendReminders: reminderModule.SendDailyDigest}),
		Contacts:      contacts.New(contacts.Deps{DB: db, Config: cfg}),
		Notes:         notes.New(notes.Deps{DB: db, Config: cfg}),
		Activities:    activities.New(activities.Deps{DB: db, Config: cfg}),
		Reminders:     reminderModule,
		Relationships: relationships.New(relationships.Deps{DB: db, Config: cfg}),
		Photos:        photos.New(photos.Deps{DB: db, Config: cfg}),
		Trash:         trash.New(trash.Deps{DB: db, Config: cfg}),
		Tokens:        tokens.New(tokens.Deps{DB: db}),
		Webhooks:      webhooks.New(webhooks.Deps{DB: db, Config: cfg}),
		Exports:       exports.New(exports.Deps{DB: db, Config: cfg}),
		Imports:       imports.New(imports.Deps{DB: db, Config: cfg}),
		MonicaImport:  monicaimport.New(monicaimport.Deps{DB: db, Config: cfg}),
		Calendars:     calendars.New(calendars.Deps{DB: db, Config: cfg}),
		ContactSync:   contactsync.New(contactsync.Deps{DB: db, Config: cfg}),
		Users:         users.New(users.Deps{DB: db, Config: cfg, OIDC: oidcProvider}),
	}
}

// Groups are the Gin groups huma operations can be registered on. A nil group means the
// API's default one; cmd/openapi passes none.
type Groups struct {
	// Public has no authentication; operations on it bring their own rate limit.
	Public gin.IRoutes
	// Protected carries authentication and the general rate limit.
	Protected gin.IRoutes
	// Admin is Protected plus the admin check, which also refuses API tokens.
	Admin gin.IRoutes
}

// RegisterAPI registers every huma operation on a. It is also what cmd/openapi calls to
// write the OpenAPI document, so it must not need a database or any running service.
// Operations are registered unconditionally (cmd/openapi passes an empty config): a feature
// that can be switched off registers its operation anyway and the handler answers as the
// disabled feature would. Do not register an operation inside an `if cfg.X`.
func RegisterAPI(a huma.API, cfg *config.Config, m Modules, g Groups) {
	m.Contacts.Register(a, g.Protected)
	m.Notes.Register(a, g.Protected)
	m.Activities.Register(a, g.Protected)
	m.Reminders.Register(a, g.Protected)
	m.Relationships.Register(a, g.Protected)
	m.Photos.Register(a, g.Protected)
	m.Trash.Register(a, g.Protected)
	m.Tokens.Register(a, g.Public, g.Protected)
	m.Webhooks.Register(a, g.Protected)
	m.Exports.Register(g.Protected)
	m.Imports.Register(a, g.Protected)
	m.MonicaImport.Register(a, g.Protected)
	m.Calendars.Register(a, g.Protected)
	m.ContactSync.Register(a, g.Protected)
	m.Users.Register(a, g.Public, g.Protected)
	m.Admin.Register(a, g.Admin)
}

func RegisterRoutes(router *gin.Engine, cfg *config.Config, db *gorm.DB, modules Modules) {

	// Health check endpoint (no versioning, standard practice)
	router.GET("/health", controllers.HealthCheck)

	// Prometheus metrics: off by default. With METRICS_ADDR set they are served on that
	// separate listener (see main.go) and not here.
	if cfg.MetricsEnabled && cfg.MetricsAddr == "" {
		router.GET("/metrics", gin.WrapH(metrics.Handler(metrics.Default, cfg.MetricsToken)))
	}

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		humaAPI := api.New(v1, router)

		// Protected routes (authentication required, general rate limiting)
		protected := v1.Group("/")
		protected.Use(middleware.APIRateLimitMiddleware())
		protected.Use(middleware.AuthMiddleware(cfg))
		protected.GET("/proxy/image", controllers.ProxyImage)

		// Admin routes (admin authentication required)
		adminGroup := v1.Group("/")
		adminGroup.Use(middleware.APIRateLimitMiddleware())
		adminGroup.Use(middleware.AuthMiddleware(cfg))
		adminGroup.Use(middleware.AdminMiddleware())

		RegisterAPI(humaAPI, cfg, modules, Groups{Public: v1, Protected: protected, Admin: adminGroup})
	}

	// CardDAV routes (optional, enabled via CARDDAV_ENABLED)
	if cfg.CardDAVEnabled {
		registerCardDAVRoutes(router, cfg, db)
	}
}

// registerCardDAVRoutes sets up CardDAV endpoints for contact synchronization
func registerCardDAVRoutes(router *gin.Engine, cfg *config.Config, db *gorm.DB) {
	// Well-known discovery endpoint (no auth required for discovery)
	router.GET("/.well-known/carddav", carddav.WellKnownRedirect)

	handler := carddav.NewHandler(db, cfg.ProfilePhotoDir)

	cardDAVGroup := router.Group("/carddav")
	cardDAVGroup.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	cardDAVGroup.Use(middleware.CardDAVRateLimitMiddleware())
	cardDAVGroup.Use(carddav.BasicAuthMiddleware())
	{
		ginHandler := handler.GinHandler()
		cardDAVGroup.Any("/*path", ginHandler)
		// WebDAV methods required for CardDAV
		cardDAVGroup.Handle("PROPFIND", "/*path", ginHandler)
		cardDAVGroup.Handle("REPORT", "/*path", ginHandler)
		cardDAVGroup.Handle("MKCOL", "/*path", ginHandler)
		cardDAVGroup.Handle("COPY", "/*path", ginHandler)
		cardDAVGroup.Handle("MOVE", "/*path", ginHandler)
	}
}
