package main

import (
	"context"
	"fmt"
	"meerkat/clock"
	"meerkat/config"
	"meerkat/controllers"
	"meerkat/database"
	apperrors "meerkat/errors"
	"meerkat/i18n"
	"meerkat/logger"
	"meerkat/metrics"
	"meerkat/middleware"
	"meerkat/reminders"
	"meerkat/routes"
	"meerkat/services"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// backgroundShutdownTimeout bounds the wait for startup tasks at shutdown. It adds
// to the scheduler stop (5s) and the HTTP server's own 30s limit, so the worst
// case is about 40s; a container stop grace period below that kills the process
// first, which is safe (SQLite is crash-consistent).
const backgroundShutdownTimeout = 5 * time.Second

func main() {
	// Initialize logger first
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	isPretty := os.Getenv("LOG_PRETTY")
	prettyLog := isPretty == "true" || isPretty == "1"

	// In development, use pretty logs by default
	if os.Getenv("GIN_MODE") != "release" {
		prettyLog = true
	}

	logger.InitLogger(logger.Config{
		Level:  logLevel,
		Pretty: prettyLog,
	})

	logger.Info().Msg("Loading server...")

	logger.Info().Msg("Loading configuration...")
	cfg := config.LoadConfig()

	logger.Info().Msg("Validating configuration...")
	cfg.ValidateOrPanic()

	clk, fakeClock, err := clock.FromEnv(cfg.FakeNow)
	if err != nil {
		logger.Fatal().Err(err).Msg("Invalid FAKE_NOW")
	}
	if fakeClock {
		logger.Warn().Str("fake_now", cfg.FakeNow).Msg("FAKE_NOW is set: the clock started at this time. Do not use this on a real instance")
	}

	logger.Info().Msg("Loading database and running migrations...")
	db, err := database.InitDB(cfg.DBPath)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize database")
	}
	// created_at and updated_at follow the same clock as everything else
	db.NowFunc = clk.Now

	// Thumbnails used to live in the database as base64: move them into files (idempotent,
	// before serving, so lists never miss a thumbnail).
	if n, err := services.ExportLegacyThumbnails(db, cfg.ProfilePhotoDir); err != nil {
		logger.Error().Err(err).Msg("Could not move contact thumbnails out of the database; thumbnails stay hidden until this works (is PROFILE_PHOTO_DIR writable?)")
	} else if n > 0 {
		logger.Info().Int("thumbnails", n).Msg("Moved contact thumbnails from the database into files")
	}

	if warn, users, err := controllers.PrivateURLWarning(db, cfg.PrivateURLsAllowed()); err != nil {
		logger.Warn().Err(err).Msg("Could not check the private URL setting")
	} else if warn {
		logger.Warn().Int64("users", users).Msg("Webhooks, calendar/contact sync and Monica import may reach private and loopback addresses " +
			"on an instance with several users. Set BLOCK_PRIVATE_URLS=true to block them (see docs/deployment.md)")
	}

	var metricsSrv *http.Server
	if cfg.MetricsEnabled {
		services.RegisterJobMetrics(metrics.Default, db)
		services.RegisterOutboxMetrics(metrics.Default, db)
		switch {
		case cfg.MetricsAddr != "":
			metricsSrv = metrics.NewServer(cfg.MetricsAddr, metrics.Default, cfg.MetricsToken)
		case cfg.MetricsToken == "":
			logger.Warn().Msg("METRICS_ENABLED without METRICS_TOKEN or METRICS_ADDR: /metrics is open on the main port. " +
				"Set a token, a separate listener, or block /metrics at your reverse proxy (see docs/deployment.md)")
		}
	}

	logger.Info().Msg("Initializing i18n translations...")
	if err := i18n.Init(); err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize i18n")
	}

	logger.Info().Msg("Running scheduler...")
	// Schedule the reminder task daily
	if !cfg.UseResend {
		logger.Warn().Msg("No Mails to be sent since Resend configuration is not set")
	}
	// Use rate-limited versions to prevent duplicate work during rapid restarts
	reminderJob := reminders.New(reminders.Deps{DB: db, Config: cfg})
	reminderTask := func() {
		if err := services.SendRemindersWithRateLimit(db, clk, reminderJob.SendDailyDigest); err != nil {
			logger.Error().Err(err).Msg("Error sending reminders")
		}
	}
	calendarSyncTask := func() {
		services.SyncCalendarsWithRateLimit(db, *cfg, clk)
	}
	contactSyncTask := func() {
		services.SyncContactsWithRateLimit(db, *cfg, clk)
	}
	trashPurgeTask := func() {
		if fakeClock {
			// Purges delete for good by age: a clock set into the future would erase real data
			logger.Warn().Msg("FAKE_NOW is set: skipping the purge jobs (trash, outbox, sessions, audit log)")
			return
		}
		now := clk.Now()
		n, err := services.PurgeExpiredTrash(db, cfg.TrashRetentionDays, cfg.ProfilePhotoDir, now)
		metrics.JobDone(services.JobTrashPurge, err)
		if err != nil {
			logger.Error().Err(err).Msg("Error purging expired trash")
		} else if n > 0 {
			logger.Info().Int("contacts", n).Msg("Purged expired contacts from the trash")
		}
		if n, err := services.PurgeEmailOutbox(db, now); err != nil {
			logger.Error().Err(err).Msg("Error purging the email outbox")
		} else if n > 0 {
			logger.Info().Int64("emails", n).Msg("Purged old emails from the outbox")
		}
		if n, err := services.PurgeWebhookDeliveries(db, now); err != nil {
			logger.Error().Err(err).Msg("Error purging webhook deliveries")
		} else if n > 0 {
			logger.Info().Int64("deliveries", n).Msg("Purged old webhook deliveries")
		}
		// Same daily slot: ended sessions and old audit entries
		if n, err := services.PurgeSessions(db, now); err != nil {
			logger.Error().Err(err).Msg("Error purging ended sessions")
		} else if n > 0 {
			logger.Info().Int64("sessions", n).Msg("Purged ended sessions")
		}
		if n, err := services.PurgeAuditLog(db, cfg.AuditLogRetentionDays, now); err != nil {
			logger.Error().Err(err).Msg("Error purging the audit log")
		} else if n > 0 {
			logger.Info().Int64("entries", n).Msg("Purged old audit log entries")
		}
	}
	backupTask := func() {
		b, err := services.RunScheduledBackup(db, cfg.BackupDir, cfg.ProfilePhotoDir, cfg.BackupRetentionCount, clk.Now())
		metrics.JobDone(services.JobBackup, err)
		if err != nil {
			logger.Error().Err(err).Msg("Scheduled backup failed")
			return
		}
		logger.Info().Str("backup", b.Name).Int64("bytes", b.Size).Msg("Scheduled backup created")
	}
	s, err := services.NewScheduler(cfg, services.SchedulerTasks{
		Reminders: reminderTask,
		WebhookRetries: func() {
			services.ProcessWebhookOutbox(db, *cfg, clk)
			services.ProcessEmailOutbox(db, *cfg, clk)
		},
		CalendarSync: calendarSyncTask,
		ContactSync:  contactSyncTask,
		TrashPurge:   trashPurgeTask,
		Backup:       backupTask,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to create scheduler")
	}
	// Run once on startup (rate-limited to prevent duplicates)
	bg := services.NewBackground()
	for name, task := range map[string]func(){
		"reminders":     reminderTask,
		"calendar-sync": calendarSyncTask,
		"contact-sync":  contactSyncTask,
		"trash-purge":   trashPurgeTask,
	} {
		bg.Go(name, func(ctx context.Context) {
			if ctx.Err() == nil {
				task()
			}
		})
	}
	s.Start()

	// gin.Default() equivalent, except that gin's access log skips requests whose path
	// holds a secret (the ICS feed token); LoggingMiddleware logs those redacted.
	r := gin.New()
	if cfg.MetricsEnabled {
		// First, so panics (answered 500 by Recovery) and rejected requests are counted too
		r.Use(metrics.DefaultHTTP().Middleware())
	}
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{Skip: middleware.IsSecretPath}), gin.Recovery())

	// Limit multipart form memory to 10MB to prevent DoS via large request bodies
	r.MaxMultipartMemory = 10 << 20 // 10 MB

	// For production, set FRONTEND_URL to specific origin(s) like "https://yourdomain.com"
	corsConfig := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "PROPFIND", "REPORT", "MKCOL", "COPY", "MOVE"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Depth", "If-Match", "If-None-Match"},
		ExposeHeaders:    []string{"Content-Length", "ETag"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour, // Cache preflight for 12 hours
	}

	// Handle wildcard "*" for development: allow any origin
	if cfg.FrontendURL == "*" {
		corsConfig.AllowOriginFunc = func(origin string) bool {
			return true // Allow all origins in development
		}
	} else {
		// Production: allow specific origin(s)
		corsConfig.AllowOrigins = []string{cfg.FrontendURL}
	}

	r.Use(cors.New(corsConfig))

	// Add security headers (HSTS only when serving over HTTPS, signaled by COOKIE_SECURE)
	r.Use(middleware.SecurityHeadersMiddleware(cfg.CookieSecure))

	// Add request body size limit middleware (10MB default) to prevent DoS
	r.Use(middleware.DefaultBodySizeLimitMiddleware())

	// Add request ID middleware for tracing
	r.Use(middleware.RequestIDMiddleware())

	// Take the time once per request; handlers read it with middleware.RequestNow
	r.Use(middleware.ClockMiddleware(clk))

	// Add logging middleware (after request ID)
	r.Use(middleware.LoggingMiddleware())

	// Add error handling middleware
	r.Use(apperrors.ErrorHandlerMiddleware())

	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		logger.Fatal().Err(err).Strs("proxies", cfg.TrustedProxies).Msg("Failed to set trusted proxies")
	}

	// Inject db and cfg into context
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})

	// Initialize OIDC provider if configured
	var oidcProvider *services.OIDCProvider
	if cfg.OIDC.Enabled {
		logger.Info().Str("provider", cfg.OIDC.ProviderURL).Msg("Initializing OIDC provider...")
		oidcCtx, oidcCancel := context.WithTimeout(context.Background(), 30*time.Second)
		var oidcErr error
		oidcProvider, oidcErr = services.InitOIDCProvider(oidcCtx, cfg)
		oidcCancel()
		if oidcErr != nil {
			logger.Fatal().Err(oidcErr).Msg("Failed to initialize OIDC provider")
		}
		logger.Info().Msg("OIDC provider initialized successfully")
	}

	// Register all routes from routes.go
	// Composition root: the converted modules are built here from concrete adapters
	modules := routes.NewModules(db, cfg, oidcProvider)
	modules.Imports.SetClock(clk)
	modules.MonicaImport.SetClock(clk)
	routes.RegisterRoutes(r, cfg, db, modules)

	// Create HTTP server with timeout configuration
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      r,
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.IdleTimeout) * time.Second,
	}

	logger.Info().
		Str("port", cfg.Port).
		Int("read_timeout", cfg.ReadTimeout).
		Int("write_timeout", cfg.WriteTimeout).
		Int("idle_timeout", cfg.IdleTimeout).
		Msg("Starting server")

	// Graceful shutdown handling
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal().Err(err).Msg("Failed to run server")
		}
	}()

	if metricsSrv != nil {
		// Bind before serving: a typo or a busy port must stop the start, not leave the
		// operator with metrics that silently never appear.
		ln, err := net.Listen("tcp", metricsSrv.Addr)
		if err != nil {
			logger.Fatal().Err(err).Str("addr", metricsSrv.Addr).Msg("Failed to listen for metrics (METRICS_ADDR)")
		}
		go func() {
			logger.Info().Str("addr", ln.Addr().String()).Msg("Serving metrics on a separate listener")
			if err := metricsSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
				logger.Error().Err(err).Msg("Metrics listener failed")
			}
		}()
	}

	logger.Info().Msg("Server is ready to handle requests")

	// Block until we receive a shutdown signal
	<-quit
	logger.Info().Msg("Shutting down server...")

	// Stop the scheduler first to prevent new jobs from starting
	logger.Info().Msg("Stopping scheduler...")
	s.Stop()

	// Create a deadline to wait for active requests to complete
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown of HTTP server
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error().Err(err).Msg("Server forced to shutdown")
	}

	if metricsSrv != nil {
		metricsCtx, metricsCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer metricsCancel()
		if err := metricsSrv.Shutdown(metricsCtx); err != nil {
			logger.Error().Err(err).Msg("Metrics listener forced to shutdown")
		}
	}

	// The startup tasks may still use the database: wait for them, bounded.
	if !bg.Shutdown(backgroundShutdownTimeout) {
		logger.Warn().Dur("timeout", backgroundShutdownTimeout).Msg("Startup tasks still running at shutdown")
	}

	// Close database connection
	logger.Info().Msg("Closing database connection...")
	sqlDB, err := db.DB()
	if err == nil {
		if err := sqlDB.Close(); err != nil {
			logger.Error().Err(err).Msg("Error closing database connection")
		}
	}

	logger.Info().Msg("Server exited gracefully")
}
