package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/xzdmycbx/nginxpanel-lite/internal/audit"
	"github.com/xzdmycbx/nginxpanel-lite/internal/auth"
	"github.com/xzdmycbx/nginxpanel-lite/internal/database"
	"github.com/xzdmycbx/nginxpanel-lite/internal/middleware"
	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
)

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) userCount() int64 {
	var n int64
	h.DB.Model(&models.User{}).Count(&n)
	return n
}

func validUsername(u string) bool {
	u = strings.TrimSpace(u)
	return len(u) >= 3 && len(u) <= 32
}

// errSetupDone signals the first admin already exists (lost a concurrent setup
// race); surfaced as a 403 so the second request is rejected cleanly.
var errSetupDone = errors.New("setup already completed")

// Setup creates the first admin user (first-run only).
func (h *Handler) Setup(c *gin.Context) {
	var in credentials
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if h.userCount() > 0 {
		fail(c, http.StatusForbidden, "setup_done", "系统已初始化")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if !validUsername(in.Username) {
		fail(c, http.StatusBadRequest, "bad_username", "用户名长度需为 3-32 个字符")
		return
	}
	if err := auth.ValidatePassword(in.Password, in.Username); err != nil {
		fail(c, http.StatusBadRequest, "bad_password", err.Error())
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		fail(c, http.StatusBadRequest, "bad_password", "密码无效")
		return
	}
	user := models.User{Username: in.Username, PasswordHash: hash, Role: models.RoleAdmin, SystemAdmin: true, TOTPEnabled: false}
	// Re-check the user count and create the first admin atomically: with a single
	// SQLite connection the transaction serializes concurrent /setup requests, so
	// two of them can't both pass the "no users yet" guard and create an admin
	// (the username unique index only blocks identical names, not this race).
	err = h.DB.Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&models.User{}).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return errSetupDone
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		return tx.Model(&models.Setting{}).Where("key = ?", database.SettingFirstRun).Update("value", "false").Error
	})
	if errors.Is(err, errSetupDone) {
		fail(c, http.StatusForbidden, "setup_done", "系统已初始化")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal", "创建用户失败")
		return
	}

	token, _ := auth.Issue(user, auth.StageEnroll, false, enrollTTL, h.Cfg.JWTSecret)
	h.Auth.SetSession(c, token, enrollTTL)

	audit.Set(c, &audit.Entry{Action: audit.ActSetupInit, TargetType: audit.TargetUser, TargetID: in.Username,
		Detail: "初始化管理员账号 " + in.Username, ActorUserID: &user.ID, ActorUsername: user.Username})
	c.JSON(http.StatusOK, gin.H{"next": "needs_totp_enroll"})
}

// Login verifies credentials and returns the next auth step.
func (h *Handler) Login(c *gin.Context) {
	if h.userCount() == 0 {
		fail(c, http.StatusConflict, "needs_setup", "系统尚未初始化")
		return
	}
	var in credentials
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	in.Username = strings.TrimSpace(in.Username)

	rlKey := "login:" + in.Username + ":" + c.ClientIP()
	if ok, retry := h.limiter.Allowed(rlKey); !ok {
		fail(c, http.StatusTooManyRequests, "rate_limited", fmt.Sprintf("尝试过于频繁，请 %d 秒后再试", int(retry.Seconds())+1))
		return
	}

	var user models.User
	err := h.DB.Where("username = ?", in.Username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || !auth.CheckPassword(user.PasswordHash, in.Password) {
		h.limiter.Fail(rlKey)
		detail := "登录失败：用户名或密码错误"
		entry := &audit.Entry{Action: audit.ActLoginFail, TargetType: audit.TargetAuth, TargetID: in.Username,
			Detail: detail, Result: audit.ResultError, ActorUsername: in.Username}
		if err == nil {
			entry.ActorUserID = &user.ID
		}
		audit.Set(c, entry)
		fail(c, http.StatusUnauthorized, "invalid_credentials", "用户名或密码错误")
		return
	}
	h.limiter.Reset(rlKey)

	if user.Disabled {
		audit.Set(c, &audit.Entry{Action: audit.ActLoginFail, TargetType: audit.TargetAuth, TargetID: user.Username,
			Detail: "登录失败：账号已被停用", Result: audit.ResultError, ActorUserID: &user.ID, ActorUsername: user.Username})
		fail(c, http.StatusForbidden, "account_disabled", "账号已被停用，请联系系统管理员")
		return
	}

	if !user.TOTPEnabled {
		token, _ := auth.Issue(user, auth.StageEnroll, false, enrollTTL, h.Cfg.JWTSecret)
		h.Auth.SetSession(c, token, enrollTTL)
		c.JSON(http.StatusOK, gin.H{"next": "needs_totp_enroll"})
		return
	}
	token, _ := auth.Issue(user, auth.StageTOTP, false, enrollTTL, h.Cfg.JWTSecret)
	h.Auth.SetSession(c, token, enrollTTL)
	c.JSON(http.StatusOK, gin.H{"next": "needs_totp_code"})
}

// TOTPEnroll generates a TOTP secret + QR for the current enrollment session.
func (h *Handler) TOTPEnroll(c *gin.Context) {
	claims := middleware.ClaimsFrom(c)
	var user models.User
	if err := h.DB.First(&user, claims.UserID).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "用户不存在")
		return
	}
	// Defense in depth: an already-enrolled user must never re-key TOTP via this
	// path (the StageEnroll route already blocks them, but guard explicitly).
	if user.TOTPEnabled {
		fail(c, http.StatusForbidden, "already_enrolled", "已绑定两步验证，重置请联系管理员")
		return
	}
	res, err := auth.GenerateTOTP(user.Username)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal", "生成两步验证失败")
		return
	}
	enc, err := auth.Encrypt(h.Cfg.SecretKey, res.SecretB32)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal", "加密失败")
		return
	}
	user.TOTPSecret = enc
	if !saveOr500(c, h.DB, &user) {
		return
	}
	c.JSON(http.StatusOK, res)
}

type totpCode struct {
	Code string `json:"code"`
}

// TOTPActivate confirms enrollment and starts a full session.
func (h *Handler) TOTPActivate(c *gin.Context) {
	claims := middleware.ClaimsFrom(c)
	var in totpCode
	_ = c.ShouldBindJSON(&in)
	var user models.User
	if err := h.DB.First(&user, claims.UserID).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "用户不存在")
		return
	}
	if user.TOTPEnabled {
		fail(c, http.StatusForbidden, "already_enrolled", "两步验证已绑定")
		return
	}
	// Rate-limit the binding step too (mirrors Login / TOTPVerify): a holder of a
	// StageEnroll cookie must not be able to brute-force the 6-digit code freely.
	rlKey := "totp_enroll:" + strconv.Itoa(int(user.ID)) + ":" + c.ClientIP()
	if ok, retry := h.limiter.Allowed(rlKey); !ok {
		fail(c, http.StatusTooManyRequests, "rate_limited", fmt.Sprintf("尝试过于频繁，请 %d 秒后再试", int(retry.Seconds())+1))
		return
	}
	secret, err := auth.Decrypt(h.Cfg.SecretKey, user.TOTPSecret)
	if err != nil || !auth.ValidateTOTP(in.Code, secret) {
		h.limiter.Fail(rlKey)
		fail(c, http.StatusBadRequest, "invalid_totp", "验证码错误")
		return
	}
	h.limiter.Reset(rlKey)
	user.TOTPEnabled = true
	if !saveOr500(c, h.DB, &user) {
		return
	}
	h.issueFull(c, user)
	audit.Set(c, &audit.Entry{Action: audit.ActTOTPEnroll, TargetType: audit.TargetAuth, TargetID: user.Username,
		Detail: "启用两步验证", ActorUserID: &user.ID, ActorUsername: user.Username})
	c.JSON(http.StatusOK, gin.H{"next": "done"})
}

// TOTPVerify checks the second factor on an existing login.
func (h *Handler) TOTPVerify(c *gin.Context) {
	claims := middleware.ClaimsFrom(c)
	var in totpCode
	_ = c.ShouldBindJSON(&in)
	var user models.User
	if err := h.DB.First(&user, claims.UserID).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "用户不存在")
		return
	}
	if !user.TOTPEnabled {
		fail(c, http.StatusBadRequest, "not_enrolled", "尚未绑定两步验证")
		return
	}
	rlKey := "totp:" + strconv.Itoa(int(user.ID)) + ":" + c.ClientIP()
	if ok, retry := h.limiter.Allowed(rlKey); !ok {
		fail(c, http.StatusTooManyRequests, "rate_limited", fmt.Sprintf("尝试过于频繁，请 %d 秒后再试", int(retry.Seconds())+1))
		return
	}
	secret, err := auth.Decrypt(h.Cfg.SecretKey, user.TOTPSecret)
	if err != nil || !auth.ValidateTOTP(in.Code, secret) {
		h.limiter.Fail(rlKey)
		audit.Set(c, &audit.Entry{Action: audit.ActLoginFail, TargetType: audit.TargetAuth, TargetID: user.Username,
			Detail: "登录失败：动态验证码错误", Result: audit.ResultError, ActorUserID: &user.ID, ActorUsername: user.Username})
		fail(c, http.StatusUnauthorized, "bad_totp", "验证码错误")
		return
	}
	h.limiter.Reset(rlKey)
	h.issueFull(c, user)
	audit.Set(c, &audit.Entry{Action: audit.ActLogin, TargetType: audit.TargetAuth, TargetID: user.Username,
		Detail: "登录成功", ActorUserID: &user.ID, ActorUsername: user.Username})
	c.JSON(http.StatusOK, gin.H{"next": "done"})
}

func (h *Handler) issueFull(c *gin.Context, user models.User) {
	token, _ := auth.Issue(user, auth.StageFull, true, h.Cfg.JWTTTL, h.Cfg.JWTSecret)
	h.Auth.SetSession(c, token, h.Cfg.JWTTTL)
}

// Me returns the bootstrap auth state for the SPA.
func (h *Handler) Me(c *gin.Context) {
	if h.userCount() == 0 {
		c.JSON(http.StatusOK, gin.H{"needsSetup": true, "authenticated": false})
		return
	}
	tok, err := c.Cookie(middleware.SessionCookie)
	if err != nil || tok == "" {
		c.JSON(http.StatusOK, gin.H{"needsSetup": false, "authenticated": false})
		return
	}
	claims, err := auth.Parse(tok, h.Cfg.JWTSecret)
	if err != nil || claims.Stage != auth.StageFull || !claims.TOTPPassed {
		c.JSON(http.StatusOK, gin.H{"needsSetup": false, "authenticated": false})
		return
	}
	var user models.User
	if err := h.DB.First(&user, claims.UserID).Error; err != nil || user.TokenEpoch != claims.Epoch {
		c.JSON(http.StatusOK, gin.H{"needsSetup": false, "authenticated": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"needsSetup":    false,
		"authenticated": true,
		"user": gin.H{
			"id": user.ID, "username": user.Username, "role": user.Role,
			"systemAdmin": user.SystemAdmin, "totpEnabled": user.TOTPEnabled,
		},
	})
}

// Logout clears the session cookie.
func (h *Handler) Logout(c *gin.Context) {
	claims := middleware.ClaimsFrom(c)
	h.Auth.ClearSession(c)
	audit.Set(c, &audit.Entry{Action: audit.ActLogout, TargetType: audit.TargetAuth, TargetID: claims.Username, Detail: "退出登录"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type changePassword struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

// ChangeOwnPassword lets a user change their own password.
func (h *Handler) ChangeOwnPassword(c *gin.Context) {
	claims := middleware.ClaimsFrom(c)
	var in changePassword
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	var user models.User
	if err := h.DB.First(&user, claims.UserID).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "用户不存在")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, in.OldPassword) {
		fail(c, http.StatusBadRequest, "wrong_password", "原密码错误")
		return
	}
	if err := auth.ValidatePassword(in.NewPassword, user.Username); err != nil {
		fail(c, http.StatusBadRequest, "bad_password", err.Error())
		return
	}
	hash, _ := auth.HashPassword(in.NewPassword)
	user.PasswordHash = hash
	user.TokenEpoch++ // revoke other sessions
	if !saveOr500(c, h.DB, &user) {
		return
	}
	h.issueFull(c, user) // keep current session valid with the new epoch

	audit.Set(c, &audit.Entry{Action: audit.ActUserChangePwd, TargetType: audit.TargetUser, TargetID: user.Username, Detail: "修改本人密码"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type selfTOTPInitReq struct {
	Password string `json:"password"`
}

// SelfResetTOTPInit starts a self-service TOTP re-bind: verify the current
// password, then return a fresh secret/QR stored as PENDING — the active TOTP
// keeps working until the new one is confirmed (so abandoning is safe).
func (h *Handler) SelfResetTOTPInit(c *gin.Context) {
	claims := middleware.ClaimsFrom(c)
	var in selfTOTPInitReq
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	var user models.User
	if err := h.DB.First(&user, claims.UserID).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "用户不存在")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, in.Password) {
		fail(c, http.StatusBadRequest, "wrong_password", "密码错误")
		return
	}
	res, err := auth.GenerateTOTP(user.Username)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal", "生成两步验证失败")
		return
	}
	enc, err := auth.Encrypt(h.Cfg.SecretKey, res.SecretB32)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal", "加密失败")
		return
	}
	user.TOTPPending = enc
	if !saveOr500(c, h.DB, &user) {
		return
	}
	c.JSON(http.StatusOK, res)
}

// SelfResetTOTPConfirm validates the new code against the pending secret, swaps
// it in, and bumps TokenEpoch to revoke ALL sessions (incl. the current one) so
// the user must log in again with the new authenticator.
func (h *Handler) SelfResetTOTPConfirm(c *gin.Context) {
	claims := middleware.ClaimsFrom(c)
	var in totpCode
	_ = c.ShouldBindJSON(&in)
	var user models.User
	if err := h.DB.First(&user, claims.UserID).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "用户不存在")
		return
	}
	if user.TOTPPending == "" {
		fail(c, http.StatusBadRequest, "no_pending", "请先发起重置")
		return
	}
	rlKey := "totp_rebind:" + strconv.Itoa(int(user.ID)) + ":" + c.ClientIP()
	if ok, retry := h.limiter.Allowed(rlKey); !ok {
		fail(c, http.StatusTooManyRequests, "rate_limited", fmt.Sprintf("尝试过于频繁，请 %d 秒后再试", int(retry.Seconds())+1))
		return
	}
	secret, err := auth.Decrypt(h.Cfg.SecretKey, user.TOTPPending)
	if err != nil || !auth.ValidateTOTP(in.Code, secret) {
		h.limiter.Fail(rlKey)
		fail(c, http.StatusBadRequest, "invalid_totp", "验证码错误")
		return
	}
	h.limiter.Reset(rlKey)
	user.TOTPSecret = user.TOTPPending
	user.TOTPPending = ""
	user.TOTPEnabled = true
	user.TokenEpoch++ // revoke all sessions (incl. current) — re-login required
	if !saveOr500(c, h.DB, &user) {
		return
	}
	audit.Set(c, &audit.Entry{Action: audit.ActTOTPRebind, TargetType: audit.TargetAuth, TargetID: user.Username,
		Detail: "重置本人两步验证（已吊销所有会话）", ActorUserID: &user.ID, ActorUsername: user.Username})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
