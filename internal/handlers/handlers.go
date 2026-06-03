// Package handlers implements the REST API handlers.
package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/xzdmycbx/nginxpanel-lite/internal/auth"
	"github.com/xzdmycbx/nginxpanel-lite/internal/config"
	"github.com/xzdmycbx/nginxpanel-lite/internal/middleware"
	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
	"github.com/xzdmycbx/nginxpanel-lite/internal/nginx"
	"github.com/xzdmycbx/nginxpanel-lite/internal/ssl"
)

const enrollTTL = 5 * time.Minute

// Handler holds the dependencies shared by all API handlers.
type Handler struct {
	DB      *gorm.DB
	Cfg     *config.Config
	Auth    *middleware.Auth
	Nginx   *nginx.Service
	SSL     *ssl.Manager
	Paths   nginx.Paths
	limiter *auth.Limiter
}

func New(db *gorm.DB, cfg *config.Config, authMw *middleware.Auth, ng *nginx.Service, sslMgr *ssl.Manager, paths nginx.Paths) *Handler {
	return &Handler{
		DB: db, Cfg: cfg, Auth: authMw, Nginx: ng, SSL: sslMgr, Paths: paths,
		// 5 failures within 15m -> 5m lockout, per (account/user + IP).
		limiter: auth.NewLimiter(5, 15*time.Minute, 5*time.Minute),
	}
}

func fail(c *gin.Context, status int, code, msg string) {
	c.JSON(status, gin.H{"code": code, "message": msg, "requestId": c.GetString("request_id")})
}

// saveOr500 persists v and writes a 500 on failure, returning false so the
// caller stops. Use for DB writes whose failure must not be silently swallowed.
func saveOr500(c *gin.Context, db *gorm.DB, v any) bool {
	if err := db.Save(v).Error; err != nil {
		log.Printf("[handler] db save error: %v", err)
		fail(c, http.StatusInternalServerError, "db_error", "保存失败")
		return false
	}
	return true
}

// applyErr turns an nginx ApplyError into a 400 with the nginx output, or a
// generic 500 otherwise. Returns true if an error was handled.
func applyErr(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if ae, ok := err.(*nginx.ApplyError); ok {
		// `nginx -t` output can leak container paths and the include layout; only
		// admins get the raw detail, normal users get a generic message.
		if claims := middleware.ClaimsFrom(c); claims != nil && claims.Role == models.RoleAdmin {
			c.JSON(http.StatusBadRequest, gin.H{"code": "nginx_invalid", "message": ae.Error(), "nginxOutput": ae.NginxOutput})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"code": "nginx_invalid", "message": "nginx 配置校验未通过（详情仅管理员可见）"})
		}
		return true
	}
	log.Printf("[handler] internal error: %v", err)
	fail(c, http.StatusInternalServerError, "internal", "内部错误，请查看服务端日志")
	return true
}
