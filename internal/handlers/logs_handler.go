package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xzdmycbx/nginxpanel-lite/internal/audit"
	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
)

// SiteLogs returns the tail of a site's access or error log (any logged-in user).
func (h *Handler) SiteLogs(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	kind := c.Query("type")
	if kind == "" {
		kind = "access"
	}
	if kind != "access" && kind != "error" {
		fail(c, http.StatusBadRequest, "bad_request", "日志类型仅支持 access 或 error")
		return
	}
	out, err := h.Nginx.TailSiteLog(site.ID, kind, atoiDefault(c.Query("lines"), 200))
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal", "读取日志失败")
		return
	}
	audit.Set(c, &audit.Entry{Action: audit.ActSiteLogView, TargetType: audit.TargetSite, TargetID: idStr(site.ID),
		Detail: "查看站点日志 " + site.Name + "（" + kind + "）"})
	c.JSON(http.StatusOK, gin.H{"type": kind, "lines": out})
}

// ClearSiteLog truncates a site's access/error log in place (admin only).
func (h *Handler) ClearSiteLog(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	kind := c.Query("type")
	if kind != "access" && kind != "error" {
		fail(c, http.StatusBadRequest, "bad_request", "日志类型仅支持 access 或 error")
		return
	}
	if err := h.Nginx.ClearSiteLog(site.ID, kind); err != nil {
		fail(c, http.StatusInternalServerError, "internal", "清空日志失败")
		return
	}
	audit.Set(c, &audit.Entry{Action: audit.ActSiteLogClear, TargetType: audit.TargetSite, TargetID: idStr(site.ID),
		Detail: "清空站点日志 " + site.Name + "（" + kind + "）"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListLogs returns the global, filterable, paginated audit log (any user).
func (h *Handler) ListLogs(c *gin.Context) {
	q := h.DB.Model(&models.AuditLog{})

	if actor := c.Query("actor"); actor != "" {
		q = q.Where("actor_username LIKE ?", "%"+actor+"%")
	}
	if action := c.Query("action"); action != "" {
		q = q.Where("action = ?", action)
	}
	if tt := c.Query("targetType"); tt != "" {
		q = q.Where("target_type = ?", tt)
	}
	if from := c.Query("from"); from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			q = q.Where("created_at >= ?", t)
		}
	}
	if to := c.Query("to"); to != "" {
		if t, err := time.Parse("2006-01-02", to); err == nil {
			q = q.Where("created_at < ?", t.AddDate(0, 0, 1))
		}
	}

	page := atoiDefault(c.Query("page"), 1)
	pageSize := atoiDefault(c.Query("pageSize"), 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}

	var total int64
	q.Count(&total)

	var items []models.AuditLog
	q.Order("created_at desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&items)

	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "pageSize": pageSize})
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
