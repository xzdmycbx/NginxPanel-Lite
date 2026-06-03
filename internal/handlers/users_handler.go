package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/xzdmycbx/nginxpanel-lite/internal/audit"
	"github.com/xzdmycbx/nginxpanel-lite/internal/auth"
	"github.com/xzdmycbx/nginxpanel-lite/internal/middleware"
	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
)

type userView struct {
	ID          uint        `json:"id"`
	Username    string      `json:"username"`
	Role        models.Role `json:"role"`
	SystemAdmin bool        `json:"systemAdmin"`
	Disabled    bool        `json:"disabled"`
	TOTPEnabled bool        `json:"totpEnabled"`
	CreatedAt   string      `json:"createdAt"`
}

func toUserView(u models.User) userView {
	return userView{ID: u.ID, Username: u.Username, Role: u.Role, SystemAdmin: u.SystemAdmin,
		Disabled: u.Disabled, TOTPEnabled: u.TOTPEnabled, CreatedAt: u.CreatedAt.Format("2006-01-02 15:04:05")}
}

// actor loads the authenticated user; needed because SystemAdmin isn't in the JWT.
func (h *Handler) actor(c *gin.Context) (*models.User, bool) {
	claims := middleware.ClaimsFrom(c)
	if claims == nil {
		fail(c, http.StatusUnauthorized, "no_session", "请先登录")
		return nil, false
	}
	var u models.User
	if err := h.DB.First(&u, claims.UserID).Error; err != nil {
		fail(c, http.StatusUnauthorized, "no_session", "请先登录")
		return nil, false
	}
	return &u, true
}

// canManage reports whether actor may run user-management actions (reset
// password/TOTP, delete) on target. Self is excluded (use the personal page);
// the system admin manages anyone, a regular admin manages only plain users.
func canManage(actor, target *models.User) bool {
	if actor.ID == target.ID {
		return false
	}
	if actor.SystemAdmin {
		return true
	}
	return target.Role == models.RoleUser && !target.SystemAdmin
}

// ListUsers returns all users (admin only).
func (h *Handler) ListUsers(c *gin.Context) {
	var users []models.User
	h.DB.Order("id asc").Find(&users)
	out := make([]userView, 0, len(users))
	for _, u := range users {
		out = append(out, toUserView(u))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

type createUserReq struct {
	Username string      `json:"username"`
	Password string      `json:"password"`
	Role     models.Role `json:"role"`
}

// CreateUser adds a new user (admin only).
func (h *Handler) CreateUser(c *gin.Context) {
	var in createUserReq
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	if !validUsername(in.Username) {
		fail(c, http.StatusBadRequest, "bad_username", "用户名长度需为 3-32 个字符")
		return
	}
	if in.Role != models.RoleAdmin && in.Role != models.RoleUser {
		in.Role = models.RoleUser
	}
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	if in.Role == models.RoleAdmin && !actor.SystemAdmin {
		fail(c, http.StatusForbidden, "forbidden", "只有系统管理员可以创建管理员")
		return
	}
	if err := auth.ValidatePassword(in.Password, in.Username); err != nil {
		fail(c, http.StatusBadRequest, "bad_password", err.Error())
		return
	}
	hash, _ := auth.HashPassword(in.Password)
	user := models.User{Username: in.Username, PasswordHash: hash, Role: in.Role}
	if err := h.DB.Create(&user).Error; err != nil {
		fail(c, http.StatusConflict, "username_taken", "用户名已存在")
		return
	}
	roleName := "普通用户"
	if in.Role == models.RoleAdmin {
		roleName = "管理员"
	}
	audit.Set(c, &audit.Entry{Action: audit.ActUserCreate, TargetType: audit.TargetUser, TargetID: in.Username,
		Detail: "创建用户 " + in.Username + "（角色：" + roleName + "）"})
	c.JSON(http.StatusOK, toUserView(user))
}

type setPasswordReq struct {
	NewPassword string `json:"newPassword"`
}

// AdminSetPassword resets another user's password (admin only; see canManage).
func (h *Handler) AdminSetPassword(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id := c.Param("id")
	var user models.User
	if err := h.DB.First(&user, id).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "用户不存在")
		return
	}
	if !canManage(actor, &user) {
		fail(c, http.StatusForbidden, "forbidden", "无权重置该账号的密码（管理员请在个人页面修改本人密码）")
		return
	}
	var in setPasswordReq
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if err := auth.ValidatePassword(in.NewPassword, user.Username); err != nil {
		fail(c, http.StatusBadRequest, "bad_password", err.Error())
		return
	}
	hash, _ := auth.HashPassword(in.NewPassword)
	user.PasswordHash = hash
	user.TokenEpoch++ // revoke target's sessions
	if !saveOr500(c, h.DB, &user) {
		return
	}
	audit.Set(c, &audit.Entry{Action: audit.ActUserResetPwd, TargetType: audit.TargetUser, TargetID: user.Username,
		Detail: "重置用户密码 " + user.Username})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AdminResetTOTP clears a user's TOTP so they must re-enroll on next login
// (admin only; see canManage). The recovery path for a lost authenticator.
func (h *Handler) AdminResetTOTP(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id := c.Param("id")
	var user models.User
	if err := h.DB.First(&user, id).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "用户不存在")
		return
	}
	if !canManage(actor, &user) {
		fail(c, http.StatusForbidden, "forbidden", "无权重置该账号的两步验证（管理员请在个人页面重置本人 TOTP）")
		return
	}
	user.TOTPEnabled = false
	user.TOTPSecret = ""
	user.TokenEpoch++ // revoke any active sessions
	if !saveOr500(c, h.DB, &user) {
		return
	}
	audit.Set(c, &audit.Entry{Action: audit.ActUserResetTOTP, TargetType: audit.TargetUser, TargetID: user.Username,
		Detail: "重置两步验证 " + user.Username + "（下次登录需重新绑定）"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteUser removes a user (admin only; see canManage). Cannot delete the
// system admin, self, or the last admin.
func (h *Handler) DeleteUser(c *gin.Context) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	id := c.Param("id")
	var user models.User
	if err := h.DB.First(&user, id).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "用户不存在")
		return
	}
	if user.SystemAdmin {
		fail(c, http.StatusForbidden, "forbidden", "不能删除系统管理员")
		return
	}
	if !canManage(actor, &user) {
		fail(c, http.StatusForbidden, "forbidden", "无权删除该账号")
		return
	}
	if user.Role == models.RoleAdmin {
		var admins int64
		h.DB.Model(&models.User{}).Where("role = ?", models.RoleAdmin).Count(&admins)
		if admins <= 1 {
			fail(c, http.StatusBadRequest, "last_admin", "不能删除最后一个管理员")
			return
		}
	}
	h.DB.Delete(&user)
	audit.Set(c, &audit.Entry{Action: audit.ActUserDelete, TargetType: audit.TargetUser, TargetID: user.Username,
		Detail: "删除用户 " + user.Username})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DisableUser blocks an account from logging in and revokes its sessions.
// EnableUser reverses it. Both are system-admin only (route-gated); cannot
// target self or the system admin.
func (h *Handler) DisableUser(c *gin.Context) { h.setUserDisabled(c, true) }
func (h *Handler) EnableUser(c *gin.Context)  { h.setUserDisabled(c, false) }

func (h *Handler) setUserDisabled(c *gin.Context, disabled bool) {
	claims := middleware.ClaimsFrom(c)
	var user models.User
	if err := h.DB.First(&user, c.Param("id")).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "用户不存在")
		return
	}
	if disabled {
		if claims != nil && user.ID == claims.UserID {
			fail(c, http.StatusBadRequest, "cannot_disable_self", "不能停用自己")
			return
		}
		if user.SystemAdmin {
			fail(c, http.StatusForbidden, "forbidden", "不能停用系统管理员")
			return
		}
	}
	if user.Disabled == disabled {
		c.JSON(http.StatusOK, gin.H{"ok": true}) // already in target state
		return
	}
	user.Disabled = disabled
	if disabled {
		user.TokenEpoch++ // kick active sessions immediately
	}
	if !saveOr500(c, h.DB, &user) {
		return
	}
	action, verb := audit.ActUserEnable, "启用账号"
	if disabled {
		action, verb = audit.ActUserDisable, "停用账号"
	}
	audit.Set(c, &audit.Entry{Action: action, TargetType: audit.TargetUser, TargetID: user.Username,
		Detail: verb + " " + user.Username})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
