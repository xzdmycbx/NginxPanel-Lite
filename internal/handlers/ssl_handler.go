package handlers

import (
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/xzdmycbx/nginxpanel-lite/internal/audit"
	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
	"github.com/xzdmycbx/nginxpanel-lite/internal/ssl"
)

type sslView struct {
	Mode          models.SSLMode `json:"mode"`
	Domains       []string       `json:"domains"`
	NotAfter      *time.Time     `json:"notAfter"`
	DaysLeft      *int           `json:"daysLeft"`
	Issuer        string         `json:"issuer"`
	Env           string         `json:"env"`
	LastRenewedAt *time.Time     `json:"lastRenewedAt"`
	RenewError    string         `json:"renewError"`
}

// GetSSL returns the SSL status for a site.
func (h *Handler) GetSSL(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	v := sslView{
		Mode:          site.SSLMode,
		Domains:       site.ServerNames,
		NotAfter:      site.CertNotAfter,
		Env:           site.ACMEEnv,
		LastRenewedAt: site.LastRenewedAt,
		RenewError:    site.RenewError,
	}
	if site.CertPath != "" {
		if info, err := readCertInfo(site.CertPath); err == nil {
			v.NotAfter = &info.NotAfter
			v.Issuer = info.Issuer
		}
	}
	if v.NotAfter != nil {
		d := int(time.Until(*v.NotAfter).Hours() / 24)
		v.DaysLeft = &d
	}
	c.JSON(http.StatusOK, v)
}

type manualSSLReq struct {
	CertPem string `json:"certPem"`
	KeyPem  string `json:"keyPem"`
}

// ManualSSL stores an uploaded cert/key pair and enables TLS.
func (h *Handler) ManualSSL(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	var in manualSSLReq
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	info, err := ssl.StoreManualCert(h.Cfg.CertsDir, site.ID, []byte(in.CertPem), []byte(in.KeyPem))
	if err != nil {
		fail(c, http.StatusBadRequest, "bad_cert", err.Error())
		return
	}
	site.SSLMode = models.SSLManual
	site.CertPath, site.KeyPath = info.CertPath, info.KeyPath
	site.CertNotAfter = &info.NotAfter
	site.ACMEEnv = ""
	site.RenewError = ""
	site.RawEdited = false // re-render overwrites manual edits

	if site.Enabled {
		if err := h.Nginx.ApplySite(c.Request.Context(), site, true); err != nil {
			applyErr(c, err)
			return
		}
	}
	if !saveOr500(c, h.DB, site) {
		return
	}
	audit.Set(c, &audit.Entry{Action: audit.ActSSLManual, TargetType: audit.TargetCert, TargetID: idStr(site.ID),
		Detail: "上传手动证书 " + site.Name})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type acmeSSLReq struct {
	Email string `json:"email"`
	Env   string `json:"env"`
}

// ACMESSL issues a Let's Encrypt certificate via HTTP-01 and enables TLS.
func (h *Handler) ACMESSL(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	var in acmeSSLReq
	_ = c.ShouldBindJSON(&in)
	h.issueACME(c, site, in.Email, in.Env, audit.ActSSLACMEIssue, "申请证书")
}

// RenewSSL force-renews an ACME certificate.
func (h *Handler) RenewSSL(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	if site.SSLMode != models.SSLACME {
		fail(c, http.StatusConflict, "not_acme", "该站点未使用 Let's Encrypt")
		return
	}
	h.issueACME(c, site, site.ACMEEmail, site.ACMEEnv, audit.ActSSLACMERenew, "续期证书")
}

func (h *Handler) issueACME(c *gin.Context, site *models.Site, email, env, action, verb string) {
	ctx := c.Request.Context()
	site.ACMEEmail = email
	site.ACMEEnv = h.SSL.ResolveEnv(env)

	// Phase A: ensure nginx serves the ACME webroot on :80 for these domains.
	if err := h.Nginx.EnsureChallengeServer(ctx, site); err != nil {
		applyErr(c, err)
		return
	}
	// Phase B: obtain the certificate.
	info, err := h.SSL.Issue(site.ServerNames, email, site.ACMEEnv, site.ID)
	if err != nil {
		site.RenewError = err.Error()
		h.DB.Save(site)
		audit.Set(c, &audit.Entry{Action: action, TargetType: audit.TargetCert, TargetID: idStr(site.ID),
			Detail: verb + "失败 " + site.Name + "：" + err.Error(), Result: audit.ResultError})
		fail(c, http.StatusBadGateway, "acme_failed", err.Error())
		return
	}
	now := time.Now()
	site.SSLMode = models.SSLACME
	site.CertPath, site.KeyPath = info.CertPath, info.KeyPath
	site.CertNotAfter = &info.NotAfter
	site.LastRenewedAt = &now
	site.RenewError = ""
	site.RawEdited = false // re-render overwrites manual edits

	// Phase C: upgrade to TLS. The cert is already issued; if reload fails we
	// still persist the cert metadata with the failure recorded.
	if err := h.Nginx.ApplySite(ctx, site, true); err != nil {
		site.RenewError = "证书已签发，但 nginx 重载失败：" + err.Error()
		h.DB.Save(site)
		applyErr(c, err)
		return
	}
	if !saveOr500(c, h.DB, site) {
		return
	}
	audit.Set(c, &audit.Entry{Action: action, TargetType: audit.TargetCert, TargetID: idStr(site.ID),
		Detail: verb + "成功 " + site.Name + "（" + site.ACMEEnv + "），有效期至 " + info.NotAfter.Format("2006-01-02")})
	c.JSON(http.StatusOK, gin.H{"ok": true, "notAfter": info.NotAfter})
}

// DisableSSL turns off TLS for a site (cert files are kept on disk).
func (h *Handler) DisableSSL(c *gin.Context) {
	site, ok := h.loadSite(c)
	if !ok {
		return
	}
	site.SSLMode = models.SSLNone
	site.CertPath, site.KeyPath = "", ""
	site.RawEdited = false // re-render overwrites manual edits
	if site.Enabled {
		if err := h.Nginx.ApplySite(c.Request.Context(), site, false); err != nil {
			applyErr(c, err)
			return
		}
	}
	if !saveOr500(c, h.DB, site) {
		return
	}
	audit.Set(c, &audit.Entry{Action: audit.ActSSLDisable, TargetType: audit.TargetCert, TargetID: idStr(site.ID),
		Detail: "关闭 SSL " + site.Name})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func readCertInfo(path string) (*ssl.CertInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ssl.ParseCertInfo(b)
}
