// Package router wires the full /api route map with per-group middleware.
package router

import (
	"github.com/gin-gonic/gin"

	"github.com/xzdmycbx/nginxpanel-lite/internal/audit"
	"github.com/xzdmycbx/nginxpanel-lite/internal/auth"
	"github.com/xzdmycbx/nginxpanel-lite/internal/handlers"
	"github.com/xzdmycbx/nginxpanel-lite/internal/middleware"
)

// Mount registers all API routes onto the engine.
func Mount(r *gin.Engine, h *handlers.Handler, a *middleware.Auth, rec *audit.Recorder) {
	api := r.Group("/api")
	api.Use(middleware.AuditMiddleware(rec))

	// Public.
	api.GET("/system/health", h.Health)
	api.GET("/system/ready", h.Ready)
	api.GET("/auth/me", h.Me)
	api.POST("/setup", h.Setup)
	api.POST("/auth/login", h.Login)

	// Enrollment endpoints: ONLY a password-verified, not-yet-enrolled session
	// (StageEnroll). This prevents an already-enrolled user (who only holds a
	// StageTOTP cookie from a password login) from overwriting their TOTP secret.
	enroll := api.Group("/auth/totp")
	enroll.Use(a.RequireStage(auth.StageEnroll))
	enroll.POST("/enroll", h.TOTPEnroll)
	enroll.POST("/activate", h.TOTPActivate)

	// Verification endpoint: ONLY a password-verified, already-enrolled session.
	verify := api.Group("/auth/totp")
	verify.Use(a.RequireStage(auth.StageTOTP))
	verify.POST("/verify", h.TOTPVerify)

	// Fully authenticated (any logged-in user).
	authed := api.Group("")
	authed.Use(a.RequireAuth())
	authed.POST("/auth/logout", h.Logout)
	authed.PUT("/me/password", h.ChangeOwnPassword)
	authed.GET("/logs", h.ListLogs)

	authed.GET("/sites", h.ListSites)
	authed.POST("/sites", h.CreateSite)
	authed.GET("/sites/:id", h.GetSite)
	authed.PUT("/sites/:id", h.UpdateSite)
	authed.DELETE("/sites/:id", h.DeleteSite)
	authed.POST("/sites/:id/toggle", h.ToggleSite)
	authed.POST("/sites/:id/preview", h.PreviewSite)
	authed.GET("/sites/:id/files", h.ListSiteFiles)
	authed.GET("/sites/:id/file", h.GetSiteFile)
	authed.GET("/sites/:id/backups", h.ListSiteBackups)
	authed.GET("/sites/:id/logs", h.SiteLogs)

	authed.GET("/sites/:id/ssl", h.GetSSL)
	authed.POST("/sites/:id/ssl/manual", h.ManualSSL)
	authed.POST("/sites/:id/ssl/acme", h.ACMESSL)
	authed.POST("/sites/:id/ssl/renew", h.RenewSSL)
	authed.DELETE("/sites/:id/ssl", h.DisableSSL)

	// Admin-only: user management + raw nginx config editing (raw config can
	// expose key files / static dirs that `nginx -t` cannot catch semantically).
	admin := authed.Group("")
	admin.Use(a.RequireAdmin())
	admin.GET("/users", h.ListUsers)
	admin.POST("/users", h.CreateUser)
	admin.PUT("/users/:id/password", h.AdminSetPassword)
	admin.POST("/users/:id/totp/reset", h.AdminResetTOTP)
	admin.DELETE("/users/:id", h.DeleteUser)

	admin.PUT("/sites/:id/file", h.SaveSiteFile)
	admin.POST("/sites/:id/backups/:ts/restore", h.RestoreSiteBackup)
	admin.POST("/sites/:id/logs/clear", h.ClearSiteLog)
}
