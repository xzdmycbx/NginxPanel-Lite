// Package audit records a global, human-readable operation log. Handlers stash
// an Entry via Set(); middleware.AuditMiddleware flushes it after the request,
// auto-stamping actor / IP / time. System events (cron) use Recorder.System.
package audit

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
)

const ctxKey = "audit_entry"

// Target types.
const (
	TargetSite   = "site"
	TargetUser   = "user"
	TargetCert   = "cert"
	TargetAuth   = "auth"
	TargetSystem = "system"
)

// Action codes (stable, machine-filterable).
const (
	ActSetupInit       = "setup.init"
	ActLogin           = "auth.login"
	ActLoginFail       = "auth.login_fail"
	ActLogout          = "auth.logout"
	ActTOTPEnroll      = "auth.totp_enroll"
	ActUserCreate      = "user.create"
	ActUserDelete      = "user.delete"
	ActUserResetPwd    = "user.reset_password"
	ActUserResetTOTP   = "user.reset_totp"
	ActUserChangePwd   = "user.change_password"
	ActSiteCreate      = "site.create"
	ActSiteUpdate      = "site.update"
	ActSiteDelete      = "site.delete"
	ActSiteToggle      = "site.toggle"
	ActSiteRawEdit     = "site.raw_edit"
	ActSiteRestore     = "site.restore"
	ActSiteFileEdit    = "site.file_edit"
	ActSiteLogClear    = "site.log_clear"
	ActSSLManual       = "ssl.manual"
	ActSSLACMEIssue    = "ssl.acme_issue"
	ActSSLACMERenew    = "ssl.acme_renew"
	ActSSLDisable      = "ssl.disable"
)

const (
	ResultOK    = "ok"
	ResultError = "error"
)

// Entry is the audit intent a handler describes; actor/IP/time are filled later.
type Entry struct {
	Action     string
	TargetType string
	TargetID   string
	Detail     string // Chinese summary; MUST NOT contain passwords or TOTP secrets
	Result     string // optional; defaults from HTTP status

	// Optional actor override for public routes (login/setup) where no
	// authenticated claims exist in context yet.
	ActorUserID   *uint
	ActorUsername string
}

// Set stashes an audit entry on the request context.
func Set(c *gin.Context, e *Entry) { c.Set(ctxKey, e) }

// Get retrieves a stashed entry.
func Get(c *gin.Context) (*Entry, bool) {
	v, ok := c.Get(ctxKey)
	if !ok {
		return nil, false
	}
	e, ok := v.(*Entry)
	return e, ok
}

// Recorder writes audit rows.
type Recorder struct{ DB *gorm.DB }

func NewRecorder(db *gorm.DB) *Recorder { return &Recorder{DB: db} }

// Write persists a fully-formed log row.
func (r *Recorder) Write(log *models.AuditLog) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	if log.Result == "" {
		log.Result = ResultOK
	}
	return r.DB.Create(log).Error
}

// System records an event performed by the panel itself (e.g. auto-renew).
func (r *Recorder) System(action, targetType, targetID, detail, result string) error {
	return r.Write(&models.AuditLog{
		ActorUsername: "system",
		Action:        action,
		TargetType:    targetType,
		TargetID:      targetID,
		Detail:        detail,
		Result:        result,
		IP:            "-",
	})
}
