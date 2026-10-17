package routes

import (
	"meerkat/activities"
	"meerkat/api"
	"meerkat/carddav"
	"meerkat/config"
	"meerkat/contacts"
	"meerkat/controllers"
	"meerkat/metrics"
	"meerkat/middleware"
	"meerkat/models"
	"meerkat/notes"
	"meerkat/photos"
	"meerkat/relationships"
	"meerkat/reminders"
	"meerkat/services"

	"github.com/danielgtaylor/huma/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Modules are the converted modules (docs/adr/0012-module-layout.md), built by main.go.
type Modules struct {
	Contacts      *contacts.Module
	Notes         *notes.Module
	Activities    *activities.Module
	Reminders     *reminders.Module
	Relationships *relationships.Module
	Photos        *photos.Module
}

// NewModules builds every converted module on db. db may be nil when only the operations'
// descriptions are needed.
func NewModules(db *gorm.DB, cfg *config.Config) Modules {
	return Modules{
		Contacts:      contacts.New(contacts.Deps{DB: db, Config: cfg}),
		Notes:         notes.New(notes.Deps{DB: db, Config: cfg}),
		Activities:    activities.New(activities.Deps{DB: db, Config: cfg}),
		Reminders:     reminders.New(reminders.Deps{DB: db, Config: cfg}),
		Relationships: relationships.New(relationships.Deps{DB: db, Config: cfg}),
		Photos:        photos.New(photos.Deps{DB: db, Config: cfg}),
	}
}

// Groups are the Gin groups huma operations can be registered on. A nil group means the
// API's default one; cmd/openapi passes none.
type Groups struct {
	// Protected carries authentication and the general rate limit.
	Protected gin.IRoutes
}

// RegisterAPI registers every huma operation on a. It is also what cmd/openapi calls to
// write the OpenAPI document, so it must not need a database or any running service.
// Operations are registered unconditionally (cmd/openapi passes an empty config): a feature
// that can be switched off registers its operation anyway and the handler answers as the
// disabled feature would. Do not register an operation inside an `if cfg.X`.
func RegisterAPI(a huma.API, cfg *config.Config, m Modules, g Groups) {
	controllers.RegisterOIDCConfig(a, cfg)
	m.Contacts.Register(a, g.Protected)
	m.Notes.Register(a, g.Protected)
	m.Activities.Register(a, g.Protected)
	m.Reminders.Register(a, g.Protected)
	m.Relationships.Register(a, g.Protected)
	m.Photos.Register(a, g.Protected)
}

func RegisterRoutes(router *gin.Engine, cfg *config.Config, db *gorm.DB, oidcProvider *services.OIDCProvider, modules Modules) {

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
		// Operations served by huma (the API contract) are registered where their group
		// exists, see RegisterAPI; the rest are plain Gin handlers until they are converted
		// (docs/adr/0011-api-contract-with-huma.md).
		humaAPI := api.New(v1, router)

		// OIDC routes (config is a huma operation; login/callback only when OIDC is enabled)
		if cfg.OIDC.Enabled && oidcProvider != nil {
			v1.GET("/auth/oidc/login", middleware.AuthRateLimitMiddleware(), controllers.OIDCLoginHandler(oidcProvider, cfg))
			v1.GET("/auth/oidc/callback", middleware.AuthRateLimitMiddleware(), controllers.OIDCCallbackHandler(oidcProvider, cfg))
		}

		// Public routes (no authentication required, strict rate limiting)
		v1.POST("/register", middleware.AuthRateLimitMiddleware(), middleware.ValidateJSONMiddleware(&models.UserRegistrationInput{}), controllers.RegisterUser(cfg))
		v1.POST("/login", middleware.AuthRateLimitMiddleware(), func(c *gin.Context) {
			controllers.LoginUser(c, cfg)
		})
		v1.POST("/logout", func(c *gin.Context) {
			controllers.LogoutUser(c, cfg)
		})
		v1.POST("/check-password-strength", middleware.AuthRateLimitMiddleware(), controllers.CheckPasswordStrength)
		v1.POST("/password-reset/request", middleware.AuthRateLimitMiddleware(), middleware.ValidateJSONMiddleware(&models.PasswordResetRequestInput{}), func(c *gin.Context) {
			controllers.RequestPasswordReset(c, cfg)
		})
		v1.POST("/password-reset/confirm", middleware.AuthRateLimitMiddleware(), middleware.ValidateJSONMiddleware(&models.PasswordResetConfirmInput{}), controllers.ConfirmPasswordReset)

		// Calendar feed (ICS). Public on purpose: calendar apps cannot send cookies or
		// headers, so the unguessable per-user token in the path is the only credential.
		v1.GET("/ics/:file", middleware.APIRateLimitMiddleware(), controllers.ServeIcsFeed)

		// Protected routes (authentication required, general rate limiting)
		protected := v1.Group("/")
		protected.Use(middleware.APIRateLimitMiddleware())
		protected.Use(middleware.AuthMiddleware(cfg))
		{
			protected.POST("/users/change-password", middleware.ValidateJSONMiddleware(&models.ChangePasswordInput{}), controllers.ChangePassword)
			protected.PATCH("/users/language", controllers.UpdateLanguage)
			protected.PATCH("/users/date-format", controllers.UpdateDateFormat)
			protected.GET("/users/custom-fields", controllers.GetCustomFieldNames)
			protected.PATCH("/users/custom-fields", middleware.ValidateJSONMiddleware(&models.CustomFieldNamesInput{}), controllers.UpdateCustomFieldNames)
			protected.GET("/users/enabled-contact-fields", controllers.GetEnabledContactFields)
			protected.PATCH("/users/enabled-contact-fields", middleware.ValidateJSONMiddleware(&models.EnabledContactFieldsInput{}), controllers.UpdateEnabledContactFields)
			protected.GET("/users/me", controllers.GetCurrentUser)

			// Login sessions: list, sign one out, sign out everywhere
			protected.GET("/sessions", controllers.ListSessions)
			protected.POST("/sessions/revoke-all", controllers.SignOutEverywhere)
			protected.DELETE("/sessions/:id", controllers.RevokeSession)

			// Two-factor (TOTP): status, enrol, enable, disable, new recovery codes
			protected.GET("/users/me/2fa", controllers.GetTOTPStatus)
			protected.POST("/users/me/2fa/setup", controllers.SetupTOTP)
			protected.POST("/users/me/2fa/enable", middleware.ValidateJSONMiddleware(&models.TOTPCodeInput{}), controllers.EnableTOTP)
			protected.POST("/users/me/2fa/disable", middleware.ValidateJSONMiddleware(&models.TOTPConfirmInput{}), controllers.DisableTOTP)
			protected.POST("/users/me/2fa/recovery-codes", middleware.ValidateJSONMiddleware(&models.TOTPConfirmInput{}), controllers.RegenerateRecoveryCodes)

			// Contact routes
			RegisterAPI(humaAPI, cfg, modules, Groups{Protected: protected})
			// Trash: soft-deleted contacts (restore, purge, empty)
			protected.GET("/trash/contacts", controllers.GetTrash)
			protected.DELETE("/trash/contacts", controllers.EmptyTrash)
			protected.POST("/trash/contacts/:id/restore", controllers.RestoreTrashedContact)
			protected.DELETE("/trash/contacts/:id", controllers.PurgeTrashedContact)

			// Contact import routes (CSV)
			protected.POST("/contacts/import/upload", controllers.UploadCSVForImport)
			protected.POST("/contacts/import/preview", middleware.ValidateJSONMiddleware(&models.ImportPreviewRequest{}), controllers.PreviewImport)
			protected.POST("/contacts/import/confirm", middleware.ValidateJSONMiddleware(&models.ImportConfirmRequest{}), controllers.ConfirmImport)

			// Contact import routes (VCF)
			protected.POST("/contacts/import/vcf/upload", func(c *gin.Context) {
				controllers.UploadVCFForImport(c, cfg)
			})
			protected.POST("/contacts/import/vcf/confirm", middleware.ValidateJSONMiddleware(&models.ImportConfirmRequest{}), func(c *gin.Context) {
				controllers.ConfirmVCFImport(c, cfg)
			})

			// Contact import routes (Monica CRM, via its REST API)
			protected.POST("/contacts/import/monica/connect", middleware.ValidateJSONMiddleware(&models.MonicaConnectRequest{}), func(c *gin.Context) {
				controllers.ConnectMonicaImport(c, cfg)
			})
			protected.POST("/contacts/import/monica/fetch", middleware.ValidateJSONMiddleware(&models.MonicaFetchRequest{}), controllers.StartMonicaFetch)
			protected.GET("/contacts/import/monica/status", controllers.GetMonicaImportStatus)
			protected.GET("/contacts/import/monica/preview", controllers.GetMonicaImportPreview)
			protected.POST("/contacts/import/monica/confirm", middleware.ValidateJSONMiddleware(&models.MonicaConfirmRequest{}), func(c *gin.Context) {
				controllers.ConfirmMonicaImport(c, cfg)
			})
			protected.DELETE("/contacts/import/monica/session", controllers.CancelMonicaImport)

			// Image proxy route (for fetching images from external URLs)
			protected.GET("/proxy/image", controllers.ProxyImage)

			// Export routes
			protected.GET("/export", controllers.ExportData)
			protected.GET("/export/vcf", func(c *gin.Context) {
				controllers.ExportContactsAsVCF(c, cfg.ProfilePhotoDir)
			})

			// API token routes
			protected.GET("/api-tokens", controllers.ListApiTokens)
			protected.POST("/api-tokens", middleware.ValidateJSONMiddleware(&models.ApiTokenInput{}), controllers.CreateApiToken)
			protected.DELETE("/api-tokens/:id", controllers.RevokeApiToken)

			// Calendar feed (ICS) management; the feed itself is the public /ics/:file route
			protected.GET("/ics-feed", controllers.GetIcsFeed)
			protected.POST("/ics-feed", controllers.CreateIcsFeed)
			protected.DELETE("/ics-feed", controllers.DeleteIcsFeed)

			// Webhook routes
			protected.GET("/webhooks", controllers.ListWebhooks)
			protected.POST("/webhooks", middleware.ValidateJSONMiddleware(&models.WebhookInput{}), controllers.CreateWebhook)
			protected.GET("/webhooks/:id", controllers.GetWebhook)
			protected.PUT("/webhooks/:id", middleware.ValidateJSONMiddleware(&models.WebhookInput{}), controllers.UpdateWebhook)
			protected.DELETE("/webhooks/:id", controllers.DeleteWebhook)
			protected.POST("/webhooks/:id/test", controllers.TestWebhook)
			protected.GET("/webhooks/:id/deliveries", controllers.GetWebhookDeliveries)

			// Calendar subscription routes (CalDAV/iCS activity import)
			protected.GET("/calendars", controllers.ListCalendarSubscriptions)
			protected.POST("/calendars", middleware.ValidateJSONMiddleware(&models.CalendarSubscriptionInput{}), controllers.CreateCalendarSubscription)
			protected.PUT("/calendars/:id", middleware.ValidateJSONMiddleware(&models.CalendarSubscriptionInput{}), controllers.UpdateCalendarSubscription)
			protected.DELETE("/calendars/:id", controllers.DeleteCalendarSubscription)
			protected.POST("/calendars/:id/sync", controllers.SyncCalendarSubscription)

			// CardDAV client routes (sync contacts with an external server)
			protected.GET("/carddav/connection", controllers.GetCardDAVConnection)
			protected.PUT("/carddav/connection", middleware.ValidateJSONMiddleware(&models.CardDAVConnectionInput{}), controllers.SaveCardDAVConnection)
			protected.DELETE("/carddav/connection", controllers.DeleteCardDAVConnection)
			protected.POST("/carddav/discover", middleware.ValidateJSONMiddleware(&models.CardDAVDiscoverInput{}), controllers.DiscoverCardDAVAddressBooks)
			protected.POST("/carddav/connection/sync", controllers.SyncCardDAVConnection)
		}

		// Admin routes (admin authentication required)
		admin := v1.Group("/admin")
		admin.Use(middleware.APIRateLimitMiddleware())
		admin.Use(middleware.AuthMiddleware(cfg))
		admin.Use(middleware.AdminMiddleware())
		{
			admin.GET("/system-status", controllers.SystemStatus)
			admin.GET("/audit-log", controllers.ListAuditLog)
			admin.GET("/users", controllers.ListUsers)
			admin.GET("/users/:id", controllers.GetUser)
			admin.PATCH("/users/:id", middleware.ValidateJSONMiddleware(&models.AdminUserUpdateInput{}), controllers.UpdateUser)
			admin.DELETE("/users/:id", controllers.DeleteUser)
			admin.POST("/users/:id/2fa/reset", controllers.ResetUserTOTP)
			// Backups contain every user's data: admin only (API tokens are refused too).
			admin.GET("/backups", controllers.ListBackups)
			admin.POST("/backups", controllers.CreateBackup)
			admin.GET("/backups/:name", controllers.DownloadBackup)
			admin.DELETE("/backups/:name", controllers.DeleteBackup)
			admin.POST("/trigger-reminders", func(c *gin.Context) {
				controllers.TriggerReminders(c, *cfg)
			})
		}
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
