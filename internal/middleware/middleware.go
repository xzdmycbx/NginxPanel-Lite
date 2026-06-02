// Package middleware provides the gin middleware chain: request id/logging,
// JWT auth with mandatory-TOTP enforcement, role guard, and audit flushing.
package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/xzdmycbx/nginxpanel-lite/internal/audit"
	"github.com/xzdmycbx/nginxpanel-lite/internal/auth"
	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
)

const (
	SessionCookie = "npl_session"
	ctxClaims     = "claims"
)

// Auth holds the dependencies for the auth middlewares and cookie helpers.
type Auth struct {
	Secret       []byte
	TTL          time.Duration
	CookieSecure bool
	DB           *gorm.DB
}

// RequestID assigns a correlation id to every request.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("request_id", uuid.NewString())
		c.Next()
	}
}

// SetSession writes the session JWT into an httpOnly cookie.
func (a *Auth) SetSession(c *gin.Context, token string, ttl time.Duration) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(SessionCookie, token, int(ttl.Seconds()), "/", "", a.CookieSecure, true)
}

// ClearSession removes the session cookie.
func (a *Auth) ClearSession(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(SessionCookie, "", -1, "/", "", a.CookieSecure, true)
}

func parseCookie(a *Auth, c *gin.Context) (*auth.Claims, bool) {
	tok, err := c.Cookie(SessionCookie)
	if err != nil || tok == "" {
		return nil, false
	}
	claims, err := auth.Parse(tok, a.Secret)
	if err != nil {
		return nil, false
	}
	return claims, true
}

// RequireStage accepts only tokens whose stage is in the allowed set (used by
// the TOTP enroll/verify endpoints).
func (a *Auth) RequireStage(stages ...auth.Stage) gin.HandlerFunc {
	allowed := map[auth.Stage]bool{}
	for _, s := range stages {
		allowed[s] = true
	}
	return func(c *gin.Context) {
		claims, ok := parseCookie(a, c)
		if !ok || !allowed[claims.Stage] {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "no_session", "message": "会话无效或已过期"})
			return
		}
		// Stage tokens must also be revocable: re-check the user and TokenEpoch
		// so a password/TOTP reset invalidates in-flight enroll/verify sessions.
		var u models.User
		if err := a.DB.First(&u, claims.UserID).Error; err != nil || u.TokenEpoch != claims.Epoch {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "session_revoked", "message": "会话已失效，请重新登录"})
			return
		}
		// Enforce stage/enrollment consistency.
		if claims.Stage == auth.StageEnroll && u.TOTPEnabled {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": "already_enrolled", "message": "已绑定两步验证"})
			return
		}
		if claims.Stage == auth.StageTOTP && !u.TOTPEnabled {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": "not_enrolled", "message": "尚未绑定两步验证"})
			return
		}
		c.Set(ctxClaims, claims)
		c.Next()
	}
}

// RequireAuth enforces a fully-authenticated, TOTP-passed session and applies a
// sliding-window cookie refresh. It is the hard gate for all business routes.
func (a *Auth) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := parseCookie(a, c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "no_session", "message": "请先登录"})
			return
		}
		if claims.Stage != auth.StageFull || !claims.TOTPPassed {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": "needs_totp", "message": "需要完成两步验证"})
			return
		}
		var u models.User
		if err := a.DB.First(&u, claims.UserID).Error; err != nil || u.TokenEpoch != claims.Epoch {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "session_revoked", "message": "会话已失效，请重新登录"})
			return
		}
		// Sliding refresh: reissue when close to expiry.
		if claims.ExpiresAt != nil && time.Until(claims.ExpiresAt.Time) < 30*time.Minute {
			if tok, err := auth.Issue(u, auth.StageFull, true, a.TTL, a.Secret); err == nil {
				a.SetSession(c, tok, a.TTL)
			}
		}
		c.Set(ctxClaims, claims)
		c.Next()
	}
}

// RequireAdmin must run after RequireAuth; it allows only admin users.
func (a *Auth) RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := ClaimsFrom(c)
		if claims == nil || claims.Role != models.RoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": "forbidden", "message": "需要管理员权限"})
			return
		}
		c.Next()
	}
}

// ClaimsFrom returns the authenticated claims, or nil.
func ClaimsFrom(c *gin.Context) *auth.Claims {
	v, ok := c.Get(ctxClaims)
	if !ok {
		return nil
	}
	claims, _ := v.(*auth.Claims)
	return claims
}

// AuditMiddleware flushes any audit Entry the handler stashed, stamping actor,
// IP, time and result.
func AuditMiddleware(rec *audit.Recorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		entry, ok := audit.Get(c)
		if !ok || entry == nil {
			return
		}
		log := &models.AuditLog{
			Action:     entry.Action,
			TargetType: entry.TargetType,
			TargetID:   entry.TargetID,
			Detail:     entry.Detail,
			Result:     entry.Result,
			IP:         c.ClientIP(),
			RequestID:  c.GetString("request_id"),
			CreatedAt:  time.Now(),
		}
		if claims := ClaimsFrom(c); claims != nil {
			id := claims.UserID
			log.ActorUserID = &id
			log.ActorUsername = claims.Username
		} else if entry.ActorUsername != "" {
			log.ActorUserID = entry.ActorUserID
			log.ActorUsername = entry.ActorUsername
		}
		if log.Result == "" {
			if c.Writer.Status() >= 400 {
				log.Result = audit.ResultError
			} else {
				log.Result = audit.ResultOK
			}
		}
		_ = rec.Write(log)
	}
}
