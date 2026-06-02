// Package app is the composition root: it wires config, DB, services, the gin
// router and the embedded SPA into an *http.Server plus the renewal scheduler.
package app

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xzdmycbx/nginxpanel-lite/internal/audit"
	"github.com/xzdmycbx/nginxpanel-lite/internal/config"
	"github.com/xzdmycbx/nginxpanel-lite/internal/database"
	"github.com/xzdmycbx/nginxpanel-lite/internal/handlers"
	"github.com/xzdmycbx/nginxpanel-lite/internal/middleware"
	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
	"github.com/xzdmycbx/nginxpanel-lite/internal/nginx"
	"github.com/xzdmycbx/nginxpanel-lite/internal/router"
	"github.com/xzdmycbx/nginxpanel-lite/internal/ssl"
	"github.com/xzdmycbx/nginxpanel-lite/internal/web"
	"gorm.io/gorm"
	"net/http"
)

// App bundles the HTTP server and the renewal scheduler.
type App struct {
	Server    *http.Server
	Scheduler *ssl.Scheduler
	db        *gorm.DB
}

// Close releases the database connection.
func (a *App) Close() error {
	if a.db == nil {
		return nil
	}
	sqlDB, err := a.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// New builds the application from configuration.
func New(cfg *config.Config) (*App, error) {
	db, err := database.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if err := database.Migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	if err := database.SeedSettings(db); err != nil {
		return nil, fmt.Errorf("seed: %w", err)
	}

	paths := nginx.Paths{
		ConfRoot:    cfg.NginxConfDir,
		CertsDir:    cfg.CertsDir,
		AcmeWebroot: cfg.AcmeWebroot,
		LogRoot:     filepath.Join(cfg.NginxConfDir, "logs"),
	}
	if err := paths.EnsureDirs(); err != nil {
		return nil, fmt.Errorf("ensure dirs: %w", err)
	}

	ctl, err := nginx.NewController(cfg.DockerHost, cfg.NginxContainerName, paths.MainConf(), cfg.NginxDryRun)
	if err != nil {
		return nil, err
	}
	ngService := nginx.NewService(paths, ctl)
	if err := ngService.SeedMainConfig(); err != nil {
		return nil, fmt.Errorf("seed nginx.conf: %w", err)
	}
	// Migrate/refresh the on-disk site trees to the multi-file layout.
	var existingSites []models.Site
	db.Find(&existingSites)
	ngService.ReconcileSites(existingSites)

	accountsDir := filepath.Join(cfg.DataDir, "acme", "accounts")
	sslMgr := ssl.NewManager(cfg.CertsDir, cfg.AcmeWebroot, accountsDir, cfg.ACMEEmail, cfg.ACMEStaging)

	rec := audit.NewRecorder(db)
	authMw := &middleware.Auth{Secret: cfg.JWTSecret, TTL: cfg.JWTTTL, CookieSecure: cfg.CookieSecure, DB: db}
	h := handlers.New(db, cfg, authMw, ngService, sslMgr, paths)

	scheduler := ssl.NewScheduler(db, sslMgr, ngService, rec)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.RequestID(), gin.Recovery(), gin.Logger())
	router.Mount(r, h, authMw, rec)
	r.NoRoute(web.SPAHandler())

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      120 * time.Second, // generous: ACME issuance is synchronous
		IdleTimeout:       60 * time.Second,
	}
	return &App{Server: srv, Scheduler: scheduler, db: db}, nil
}
