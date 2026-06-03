package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xzdmycbx/nginxpanel-lite/internal/audit"
	"github.com/xzdmycbx/nginxpanel-lite/internal/auth"
	"github.com/xzdmycbx/nginxpanel-lite/internal/middleware"
	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
)

// Domain names written into server_name. Allows an optional leading wildcard
// label and standard hostname characters; rejects anything that could break
// out of the directive (spaces, ; { } # etc.).
var domainRe = regexp.MustCompile(`^(\*\.)?([a-zA-Z0-9_]([a-zA-Z0-9_-]{0,61}[a-zA-Z0-9_])?\.)*[a-zA-Z0-9_]([a-zA-Z0-9_-]{0,61}[a-zA-Z0-9_])?$`)

// Characters that must never appear in a proxy_pass target (would allow nginx
// directive injection or config breakout).
var upstreamBadRe = regexp.MustCompile("[\\s;{}#\\\\\"'`$]")

// host[:port][/path] shape for scheme-less targets.
var hostPortRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]+(:[0-9]{1,5})?(/[^\s;{}#\\]*)?$`)

// Location paths may use nginx modifiers (= ~ ^~) but must not contain
// directive-breaking characters.
var locationPathBadRe = regexp.MustCompile("[;{}#\\n\\r]")

func validateLocationPath(p string) error {
	if p == "" || len(p) > 200 || locationPathBadRe.MatchString(p) {
		return fmt.Errorf("反向代理路径不合法：%s", p)
	}
	return nil
}

func validateServerName(d string) error {
	if len(d) > 253 || !domainRe.MatchString(d) {
		return fmt.Errorf("域名格式不合法：%s", d)
	}
	return nil
}

func validateUpstream(t string) error {
	if upstreamBadRe.MatchString(t) {
		return fmt.Errorf("反向代理目标包含非法字符：%s", t)
	}
	if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
		u, err := url.Parse(t)
		if err != nil || u.Host == "" {
			return fmt.Errorf("反向代理目标格式不合法：%s", t)
		}
		return nil
	}
	if !hostPortRe.MatchString(t) {
		return fmt.Errorf("反向代理目标格式不合法：%s", t)
	}
	return nil
}

type proxyLocationInput struct {
	Path             string   `json:"path"`
	UpstreamTargets  []string `json:"upstreamTargets"`
	WebsocketUpgrade bool     `json:"websocketUpgrade"`
	CacheEnabled     bool     `json:"cacheEnabled"`
	ExtraConfig      string   `json:"extraConfig"`
}

type siteReq struct {
	Name               string               `json:"name"`
	ServerNames        []string             `json:"serverNames"`
	Locations          []proxyLocationInput `json:"locations"`
	ForceHTTPSRedirect bool                 `json:"forceHttpsRedirect"`
	RawConfigOverride  string               `json:"rawConfigOverride"`
	// Legacy single-location fallback (older clients / tests).
	UpstreamTargets  []string `json:"upstreamTargets"`
	WebsocketUpgrade bool     `json:"websocketUpgrade"`
}

func (in *siteReq) toLocations() []models.ProxyLocation {
	out := make([]models.ProxyLocation, 0, len(in.Locations))
	for _, l := range in.Locations {
		out = append(out, models.ProxyLocation{
			Path:             l.Path,
			UpstreamTargets:  l.UpstreamTargets,
			WebsocketUpgrade: l.WebsocketUpgrade,
			CacheEnabled:     l.CacheEnabled,
			ExtraConfig:      l.ExtraConfig,
		})
	}
	return out
}

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (in *siteReq) normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	in.ServerNames = cleanList(in.ServerNames)
	if len(in.ServerNames) == 0 {
		return fmt.Errorf("请至少填写一个域名")
	}
	for _, d := range in.ServerNames {
		if err := validateServerName(d); err != nil {
			return err
		}
	}

	// Legacy fallback: synthesize a single "/" location from upstreamTargets.
	if len(in.Locations) == 0 && len(in.UpstreamTargets) > 0 {
		in.Locations = []proxyLocationInput{{
			Path:             "/",
			UpstreamTargets:  in.UpstreamTargets,
			WebsocketUpgrade: in.WebsocketUpgrade,
		}}
	}
	if len(in.Locations) == 0 {
		return fmt.Errorf("请至少配置一个反向代理路径")
	}
	for i := range in.Locations {
		l := &in.Locations[i]
		l.Path = strings.TrimSpace(l.Path)
		if l.Path == "" {
			l.Path = "/"
		}
		if err := validateLocationPath(l.Path); err != nil {
			return err
		}
		l.UpstreamTargets = cleanList(l.UpstreamTargets)
		if len(l.UpstreamTargets) == 0 {
			return fmt.Errorf("反向代理路径 %s 未配置目标", l.Path)
		}
		for _, u := range l.UpstreamTargets {
			if err := validateUpstream(u); err != nil {
				return err
			}
		}
	}

	if in.Name == "" {
		in.Name = in.ServerNames[0]
	}
	return nil
}

// summarizeSite builds a short Chinese description for audit diffs.
func summarizeSite(s *models.Site) string {
	parts := make([]string, 0)
	for _, l := range s.EffectiveLocations() {
		parts = append(parts, l.Path+"→"+strings.Join(l.UpstreamTargets, "|"))
	}
	return "域名 " + strings.Join(s.ServerNames, ",") + "；" + strings.Join(parts, "; ")
}

func enableTLSFor(s *models.Site) bool {
	return s.CertID != nil && s.CertPath != "" && s.KeyPath != ""
}

// certBrief is the cert summary embedded in a site response (badge / expiry).
type certBrief struct {
	ID       uint              `json:"id"`
	Name     string            `json:"name"`
	Source   models.CertSource `json:"source"`
	NotAfter *time.Time        `json:"notAfter"`
	DaysLeft *int              `json:"daysLeft"`
}

type siteView struct {
	models.Site
	Cert *certBrief `json:"cert"`
}

func (h *Handler) siteView(s *models.Site) siteView {
	v := siteView{Site: *s}
	if s.CertID != nil {
		var cert models.Certificate
		if err := h.DB.First(&cert, *s.CertID).Error; err == nil {
			b := &certBrief{ID: cert.ID, Name: cert.Name, Source: cert.Source, NotAfter: cert.NotAfter}
			if cert.NotAfter != nil {
				d := int(time.Until(*cert.NotAfter).Hours() / 24)
				b.DaysLeft = &d
			}
			v.Cert = b
		}
	}
	return v
}

// bindCert resolves certID to a Certificate and denormalizes its on-disk paths
// onto the site (nil clears the binding -> no TLS).
func (h *Handler) bindCert(site *models.Site, certID *uint) error {
	if certID == nil {
		site.CertID = nil
		site.CertPath, site.KeyPath = "", ""
		return nil
	}
	var cert models.Certificate
	if err := h.DB.First(&cert, *certID).Error; err != nil {
		return fmt.Errorf("所选证书不存在")
	}
	site.CertID = &cert.ID
	site.CertPath, site.KeyPath = cert.CertPath, cert.KeyPath
	return nil
}

type bindCertReq struct {
	CertID *uint `json:"certId"`
}

// BindSiteCert binds (or clears, certId=null) a global cert to a site and
// re-applies its nginx config.
func (h *Handler) BindSiteCert(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	if siteLocked(c, site) {
		return
	}
	var in bindCertReq
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if err := h.bindCert(site, in.CertID); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	site.RawEdited = false // re-render overwrites manual edits
	if site.Enabled {
		if err := h.Nginx.ApplySite(c.Request.Context(), site, enableTLSFor(site)); err != nil {
			applyErr(c, err)
			return
		}
	}
	if !saveOr500(c, h.DB, site) {
		return
	}
	detail := "解绑 SSL 证书 " + site.Name
	if site.CertID != nil {
		detail = "绑定 SSL 证书 " + site.Name
	}
	audit.Set(c, &audit.Entry{Action: audit.ActSiteBindCert, TargetType: audit.TargetSite, TargetID: idStr(site.ID),
		Detail: detail})
	c.JSON(http.StatusOK, h.siteView(site))
}

// ListSites returns all sites (with bound-cert summary).
func (h *Handler) ListSites(c *gin.Context) {
	var sites []models.Site
	h.DB.Order("id asc").Find(&sites)
	out := make([]siteView, 0, len(sites))
	for i := range sites {
		out = append(out, h.siteView(&sites[i]))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func (h *Handler) loadSite(c *gin.Context) (*models.Site, bool) {
	var site models.Site
	if err := h.DB.First(&site, c.Param("id")).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "站点不存在")
		return nil, false
	}
	return &site, true
}

// siteLocked rejects (and returns true) if the site is locked. A locked site is
// frozen for everyone — only the system admin's lock/unlock may change it.
func siteLocked(c *gin.Context, site *models.Site) bool {
	if site.Locked {
		fail(c, http.StatusForbidden, "site_locked", "站点已被系统管理员锁定，请先解锁后再操作")
		return true
	}
	return false
}

type siteLockReq struct {
	Code string `json:"code"` // system admin's TOTP, required to lock/unlock
}

// LockSite/UnlockSite freeze/unfreeze a site. System-admin only (route-gated),
// and each requires the admin's current TOTP code.
func (h *Handler) LockSite(c *gin.Context)   { h.setSiteLocked(c, true) }
func (h *Handler) UnlockSite(c *gin.Context) { h.setSiteLocked(c, false) }

func (h *Handler) setSiteLocked(c *gin.Context, locked bool) {
	actor, ok := h.actor(c)
	if !ok {
		return
	}
	var in siteLockReq
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	rlKey := "site_lock:" + idStr(actor.ID) + ":" + c.ClientIP()
	if okRate, retry := h.limiter.Allowed(rlKey); !okRate {
		fail(c, http.StatusTooManyRequests, "rate_limited", fmt.Sprintf("尝试过于频繁，请 %d 秒后再试", int(retry.Seconds())+1))
		return
	}
	secret, err := auth.Decrypt(h.Cfg.SecretKey, actor.TOTPSecret)
	if err != nil || !auth.ValidateTOTP(in.Code, secret) {
		h.limiter.Fail(rlKey)
		fail(c, http.StatusBadRequest, "invalid_totp", "两步验证码错误")
		return
	}
	h.limiter.Reset(rlKey)

	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	if site.Locked == locked {
		c.JSON(http.StatusOK, h.siteView(site)) // already in target state
		return
	}
	site.Locked = locked
	if !saveOr500(c, h.DB, site) {
		return
	}
	action, verb := audit.ActSiteUnlock, "解锁站点"
	if locked {
		action, verb = audit.ActSiteLock, "锁定站点"
	}
	audit.Set(c, &audit.Entry{Action: action, TargetType: audit.TargetSite, TargetID: idStr(site.ID),
		Detail: verb + " " + site.Name})
	c.JSON(http.StatusOK, h.siteView(site))
}

// GetSite returns one site (with bound-cert summary).
func (h *Handler) GetSite(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, h.siteView(site))
}

// CreateSite creates a reverse-proxy site and applies its config.
func (h *Handler) CreateSite(c *gin.Context) {
	claims := middleware.ClaimsFrom(c)
	var in siteReq
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if err := in.normalize(); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	uid := claims.UserID
	// Raw nginx config is admin-only; strip it from non-admin submissions.
	locs := in.toLocations()
	raw := in.RawConfigOverride
	if claims.Role != models.RoleAdmin {
		raw = ""
		for i := range locs {
			locs[i].ExtraConfig = ""
		}
	}
	site := models.Site{
		Name:               in.Name,
		ServerNames:        in.ServerNames,
		Locations:          locs,
		ForceHTTPSRedirect: in.ForceHTTPSRedirect,
		RawConfigOverride:  raw,
		Enabled:            true,
		UpdatedByUserID:    &uid,
	}
	if err := h.DB.Create(&site).Error; err != nil {
		fail(c, http.StatusInternalServerError, "internal", "保存站点失败")
		return
	}
	// New sites have no certificate; SSL is bound later from the site SSL tab.
	if err := h.Nginx.ApplySite(c.Request.Context(), &site, false); err != nil {
		h.DB.Delete(&site) // keep DB and nginx consistent
		applyErr(c, err)
		return
	}
	audit.Set(c, &audit.Entry{Action: audit.ActSiteCreate, TargetType: audit.TargetSite, TargetID: idStr(site.ID),
		Detail: "创建站点 " + strings.Join(site.ServerNames, ", ")})
	c.JSON(http.StatusOK, h.siteView(&site))
}

// UpdateSite updates a site and re-applies its config.
func (h *Handler) UpdateSite(c *gin.Context) {
	claims := middleware.ClaimsFrom(c)
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	if siteLocked(c, site) {
		return
	}
	var in siteReq
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	if err := in.normalize(); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	before := summarizeSite(site)
	uid := claims.UserID
	isAdmin := claims.Role == models.RoleAdmin
	newLocs := in.toLocations()
	if !isAdmin {
		// Non-admins cannot edit raw config; preserve any admin-set values.
		oldExtra := map[string]string{}
		for _, l := range site.EffectiveLocations() {
			oldExtra[l.Path] = l.ExtraConfig
		}
		for i := range newLocs {
			newLocs[i].ExtraConfig = oldExtra[newLocs[i].Path]
		}
	}
	site.Name = in.Name
	site.ServerNames = in.ServerNames
	site.Locations = newLocs
	site.UpstreamTargets = nil // clear legacy single-location fields
	site.WebsocketUpgrade = false
	site.ForceHTTPSRedirect = in.ForceHTTPSRedirect
	if isAdmin {
		site.RawConfigOverride = in.RawConfigOverride
	}
	site.RawEdited = false // structured save regenerates all files
	site.UpdatedByUserID = &uid

	// Apply to nginx BEFORE persisting: on validation/reload failure the DB keeps
	// the previous (working) values, staying consistent with the live config.
	if site.Enabled {
		if err := h.Nginx.ApplySite(c.Request.Context(), site, enableTLSFor(site)); err != nil {
			applyErr(c, err)
			return
		}
	}
	if !saveOr500(c, h.DB, site) {
		return
	}

	after := summarizeSite(site)
	audit.Set(c, &audit.Entry{Action: audit.ActSiteUpdate, TargetType: audit.TargetSite, TargetID: idStr(site.ID),
		Detail: "修改站点 " + site.Name + "：" + before + " ⇒ " + after})
	c.JSON(http.StatusOK, h.siteView(site))
}

// DeleteSite removes a site and its nginx config.
func (h *Handler) DeleteSite(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	if siteLocked(c, site) {
		return
	}
	if err := h.Nginx.RemoveSite(c.Request.Context(), site.ID); err != nil {
		applyErr(c, err)
		return
	}
	h.DB.Delete(site)
	audit.Set(c, &audit.Entry{Action: audit.ActSiteDelete, TargetType: audit.TargetSite, TargetID: idStr(site.ID),
		Detail: "删除站点 " + site.Name})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ToggleSite enables/disables a site.
func (h *Handler) ToggleSite(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	if siteLocked(c, site) {
		return
	}
	site.Enabled = !site.Enabled
	if site.Enabled {
		site.RawEdited = false // re-render overwrites any manual edits
		if err := h.Nginx.ApplySite(c.Request.Context(), site, enableTLSFor(site)); err != nil {
			applyErr(c, err)
			return
		}
	} else {
		if err := h.Nginx.RemoveSite(c.Request.Context(), site.ID); err != nil {
			applyErr(c, err)
			return
		}
	}
	if !saveOr500(c, h.DB, site) {
		return
	}
	state := "启用"
	if !site.Enabled {
		state = "停用"
	}
	audit.Set(c, &audit.Entry{Action: audit.ActSiteToggle, TargetType: audit.TargetSite, TargetID: idStr(site.ID),
		Detail: state + "站点 " + site.Name})
	c.JSON(http.StatusOK, site)
}

type rawConfigReq struct {
	Content string `json:"content"`
}

// PreviewSite renders the would-be config (dry-run) for viewing.
func (h *Handler) PreviewSite(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	files, err := h.Nginx.Render(site, enableTLSFor(site))
	if err != nil {
		fail(c, http.StatusBadRequest, "render_error", err.Error())
		return
	}
	var b strings.Builder
	b.WriteString("# ===== site.conf =====\n")
	b.Write(files.SiteConf)
	for _, l := range files.Locations {
		b.WriteString("\n# ===== locations/" + l.Slug + ".conf =====\n")
		b.Write(l.Content)
	}
	c.JSON(http.StatusOK, gin.H{"generated": b.String()})
}

// ListSiteFiles lists the editable config files of a site.
func (h *Handler) ListSiteFiles(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": h.Nginx.ListSiteFiles(site.ID), "rawEdited": site.RawEdited})
}

// GetSiteFile returns one editable file's content (?key=site|loc:<slug>).
func (h *Handler) GetSiteFile(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	key := c.Query("key")
	content, err := h.Nginx.GetSiteFile(site.ID, key)
	if err != nil {
		fail(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	audit.Set(c, &audit.Entry{Action: audit.ActSiteFileView, TargetType: audit.TargetSite, TargetID: idStr(site.ID),
		Detail: "查看配置文件 " + site.Name + "（" + key + "）"})
	c.JSON(http.StatusOK, gin.H{"content": content})
}

// SaveSiteFile applies a hand-edited config file (admin) through the nginx gate.
func (h *Handler) SaveSiteFile(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	if siteLocked(c, site) {
		return
	}
	var in rawConfigReq
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	key := c.Query("key")
	if err := h.Nginx.SaveSiteFile(c.Request.Context(), site.ID, key, []byte(in.Content)); err != nil {
		applyErr(c, err)
		return
	}
	site.RawEdited = true
	if !saveOr500(c, h.DB, site) {
		return
	}
	audit.Set(c, &audit.Entry{Action: audit.ActSiteFileEdit, TargetType: audit.TargetSite, TargetID: idStr(site.ID),
		Detail: "编辑配置文件 " + site.Name + "（" + key + "）"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListSiteBackups lists saved config backups for a site.
func (h *Handler) ListSiteBackups(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	backups, err := h.Nginx.ListBackups(site.ID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": backups})
}

// RestoreSiteBackup restores a saved config backup.
func (h *Handler) RestoreSiteBackup(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	if siteLocked(c, site) {
		return
	}
	ts := c.Param("ts")
	if err := h.Nginx.RestoreBackup(c.Request.Context(), site.ID, ts); err != nil {
		applyErr(c, err)
		return
	}
	site.RawEdited = true // restored config may differ from the structured model
	h.DB.Save(site)
	audit.Set(c, &audit.Entry{Action: audit.ActSiteRestore, TargetType: audit.TargetSite, TargetID: idStr(site.ID),
		Detail: "恢复站点配置备份 " + site.Name + "（" + ts + "）"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func idStr(id uint) string { return fmt.Sprintf("%d", id) }
