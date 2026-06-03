package handlers

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/xzdmycbx/nginxpanel-lite/internal/audit"
	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
	"github.com/xzdmycbx/nginxpanel-lite/internal/ssl"
)

// errCertInUse signals a cert can't be deleted because a site references it
// (checked inside the delete transaction to avoid a bind/delete race).
var errCertInUse = errors.New("certificate in use")

type certView struct {
	ID            uint              `json:"id"`
	Name          string            `json:"name"`
	Source        models.CertSource `json:"source"`
	Domains       []string          `json:"domains"`
	NotAfter      *time.Time        `json:"notAfter"`
	DaysLeft      *int              `json:"daysLeft"`
	Issuer        string            `json:"issuer"`
	ACMEEmail     string            `json:"acmeEmail"`
	ACMEEnv       string            `json:"acmeEnv"`
	LastRenewedAt *time.Time        `json:"lastRenewedAt"`
	RenewError    string            `json:"renewError"`
	InUseBy       []string          `json:"inUseBy"`
	CreatedAt     string            `json:"createdAt"`
}

func toCertView(cert models.Certificate, usage map[uint][]string) certView {
	v := certView{
		ID: cert.ID, Name: cert.Name, Source: cert.Source, Domains: cert.Domains,
		NotAfter: cert.NotAfter, Issuer: cert.Issuer, ACMEEmail: cert.ACMEEmail, ACMEEnv: cert.ACMEEnv,
		LastRenewedAt: cert.LastRenewedAt, RenewError: cert.RenewError,
		InUseBy: usage[cert.ID], CreatedAt: cert.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	if v.Domains == nil {
		v.Domains = []string{}
	}
	if v.InUseBy == nil {
		v.InUseBy = []string{}
	}
	if cert.NotAfter != nil {
		d := int(time.Until(*cert.NotAfter).Hours() / 24)
		v.DaysLeft = &d
	}
	return v
}

// certUsage maps cert ID -> names of sites referencing it.
func (h *Handler) certUsage() map[uint][]string {
	var sites []models.Site
	h.DB.Where("cert_id IS NOT NULL").Find(&sites)
	out := map[uint][]string{}
	for _, s := range sites {
		if s.CertID != nil {
			out[*s.CertID] = append(out[*s.CertID], s.Name)
		}
	}
	return out
}

// ListCerts returns all global certificates with usage + expiry info.
func (h *Handler) ListCerts(c *gin.Context) {
	var certs []models.Certificate
	h.DB.Order("id asc").Find(&certs)
	usage := h.certUsage()
	out := make([]certView, 0, len(certs))
	for _, cert := range certs {
		out = append(out, toCertView(cert, usage))
	}
	c.JSON(http.StatusOK, gin.H{"items": out})
}

func validCertName(name string) bool {
	name = strings.TrimSpace(name)
	return len(name) >= 1 && len(name) <= 64
}

type manualCertReq struct {
	Name    string `json:"name"`
	CertPem string `json:"certPem"`
	KeyPem  string `json:"keyPem"`
}

// CreateManualCert validates + stores an uploaded cert/key pair as a named cert.
func (h *Handler) CreateManualCert(c *gin.Context) {
	var in manualCertReq
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !validCertName(in.Name) {
		fail(c, http.StatusBadRequest, "bad_name", "证书名称长度需为 1-64 个字符")
		return
	}
	info, err := ssl.ValidatePEMPair([]byte(in.CertPem), []byte(in.KeyPem))
	if err != nil {
		fail(c, http.StatusBadRequest, "bad_cert", err.Error())
		return
	}
	cert := models.Certificate{Name: in.Name, Source: models.CertManual, Domains: info.Domains,
		NotAfter: &info.NotAfter, Issuer: info.Issuer}
	if err := h.DB.Create(&cert).Error; err != nil {
		fail(c, http.StatusConflict, "name_taken", "证书名称已存在")
		return
	}
	certPath, keyPath := ssl.NamedCertFiles(h.Cfg.CertsDir, cert.ID)
	if _, err := ssl.StoreManualCert(certPath, keyPath, []byte(in.CertPem), []byte(in.KeyPem)); err != nil {
		h.DB.Delete(&cert)
		fail(c, http.StatusInternalServerError, "internal", "写入证书失败")
		return
	}
	cert.CertPath, cert.KeyPath = certPath, keyPath
	if !saveOr500(c, h.DB, &cert) {
		return
	}
	audit.Set(c, &audit.Entry{Action: audit.ActCertCreate, TargetType: audit.TargetCert, TargetID: idStr(cert.ID),
		Detail: "上传证书 " + cert.Name})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type acmeCertReq struct {
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
	Email   string   `json:"email"`
	Env     string   `json:"env"`
}

// IssueACMECert creates a named cert and issues it via Let's Encrypt. The main
// nginx default :80 server answers the HTTP-01 challenge for any domain.
func (h *Handler) IssueACMECert(c *gin.Context) {
	var in acmeCertReq
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if !validCertName(in.Name) {
		fail(c, http.StatusBadRequest, "bad_name", "证书名称长度需为 1-64 个字符")
		return
	}
	domains := cleanList(in.Domains)
	if len(domains) == 0 {
		fail(c, http.StatusBadRequest, "bad_request", "请至少填写一个域名")
		return
	}
	for _, d := range domains {
		if err := validateServerName(d); err != nil {
			fail(c, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
	}
	env := h.SSL.ResolveEnv(in.Env)
	cert := models.Certificate{Name: in.Name, Source: models.CertACME, Domains: domains,
		ACMEEmail: in.Email, ACMEEnv: env}
	if err := h.DB.Create(&cert).Error; err != nil {
		fail(c, http.StatusConflict, "name_taken", "证书名称已存在")
		return
	}
	certPath, keyPath := ssl.NamedCertFiles(h.Cfg.CertsDir, cert.ID)
	info, err := h.SSL.Issue(c.Request.Context(), domains, in.Email, env, certPath, keyPath)
	if err != nil {
		h.DB.Delete(&cert) // first issuance failed: no files written, drop the record
		audit.Set(c, &audit.Entry{Action: audit.ActCertIssue, TargetType: audit.TargetCert, TargetID: idStr(cert.ID),
			Detail: "申请证书失败 " + cert.Name + "：" + err.Error(), Result: audit.ResultError})
		fail(c, http.StatusBadGateway, "acme_failed", err.Error())
		return
	}
	now := time.Now()
	cert.CertPath, cert.KeyPath = certPath, keyPath
	cert.NotAfter, cert.Issuer, cert.LastRenewedAt = &info.NotAfter, info.Issuer, &now
	if !saveOr500(c, h.DB, &cert) {
		return
	}
	audit.Set(c, &audit.Entry{Action: audit.ActCertIssue, TargetType: audit.TargetCert, TargetID: idStr(cert.ID),
		Detail: "申请证书成功 " + cert.Name + "（" + env + "），有效期至 " + info.NotAfter.Format("2006-01-02")})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type updateCertReq struct {
	Name    string   `json:"name"`
	CertPem string   `json:"certPem"` // manual: re-upload (optional)
	KeyPem  string   `json:"keyPem"`
	Domains []string `json:"domains"` // acme: re-issue for these domains (optional)
	Email   string   `json:"email"`
	Env     string   `json:"env"`
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// UpdateCert renames a certificate and/or replaces its content (manual: new PEM;
// acme: re-issue for new domains/email/env). After a content change nginx is
// reloaded so every site using the cert syncs to the new files automatically.
func (h *Handler) UpdateCert(c *gin.Context) {
	var cert models.Certificate
	if err := h.DB.First(&cert, c.Param("id")).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "证书不存在")
		return
	}
	var in updateCertReq
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "bad_request", "请求格式错误")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name != "" && in.Name != cert.Name {
		if !validCertName(in.Name) {
			fail(c, http.StatusBadRequest, "bad_name", "证书名称长度需为 1-64 个字符")
			return
		}
		var n int64
		h.DB.Model(&models.Certificate{}).Where("name = ? AND id <> ?", in.Name, cert.ID).Count(&n)
		if n > 0 {
			fail(c, http.StatusConflict, "name_taken", "证书名称已存在")
			return
		}
		cert.Name = in.Name
	}

	contentChanged := false
	if cert.Source == models.CertManual {
		if strings.TrimSpace(in.CertPem) != "" || strings.TrimSpace(in.KeyPem) != "" {
			info, err := ssl.ValidatePEMPair([]byte(in.CertPem), []byte(in.KeyPem))
			if err != nil {
				fail(c, http.StatusBadRequest, "bad_cert", err.Error())
				return
			}
			if _, err := ssl.StoreManualCert(cert.CertPath, cert.KeyPath, []byte(in.CertPem), []byte(in.KeyPem)); err != nil {
				fail(c, http.StatusInternalServerError, "internal", "写入证书失败")
				return
			}
			cert.Domains, cert.NotAfter, cert.Issuer = info.Domains, &info.NotAfter, info.Issuer
			contentChanged = true
		}
	} else { // acme: re-issue if domains/email/env changed
		domains := cleanList(in.Domains)
		env := h.SSL.ResolveEnv(in.Env)
		if len(domains) > 0 && (!sameStrings(domains, cert.Domains) || in.Email != cert.ACMEEmail || env != cert.ACMEEnv) {
			for _, d := range domains {
				if err := validateServerName(d); err != nil {
					fail(c, http.StatusBadRequest, "bad_request", err.Error())
					return
				}
			}
			info, err := h.SSL.Issue(c.Request.Context(), domains, in.Email, env, cert.CertPath, cert.KeyPath)
			if err != nil {
				cert.RenewError = err.Error()
				h.DB.Save(&cert)
				fail(c, http.StatusBadGateway, "acme_failed", err.Error())
				return
			}
			now := time.Now()
			cert.Domains, cert.ACMEEmail, cert.ACMEEnv = domains, in.Email, env
			cert.NotAfter, cert.Issuer, cert.LastRenewedAt, cert.RenewError = &info.NotAfter, info.Issuer, &now, ""
			contentChanged = true
		}
	}

	if !saveOr500(c, h.DB, &cert) {
		return
	}
	if contentChanged {
		if err := h.Nginx.Reload(c.Request.Context()); err != nil { // sync every site using this cert
			applyErr(c, err)
			return
		}
	}
	audit.Set(c, &audit.Entry{Action: audit.ActCertUpdate, TargetType: audit.TargetCert, TargetID: idStr(cert.ID),
		Detail: "修改证书 " + cert.Name})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RenewCert re-issues an ACME certificate to the same paths, then reloads nginx
// so any site using it picks up the new content.
func (h *Handler) RenewCert(c *gin.Context) {
	var cert models.Certificate
	if err := h.DB.First(&cert, c.Param("id")).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "证书不存在")
		return
	}
	if cert.Source != models.CertACME {
		fail(c, http.StatusConflict, "not_acme", "该证书不是 Let's Encrypt 证书")
		return
	}
	info, err := h.SSL.Issue(c.Request.Context(), cert.Domains, cert.ACMEEmail, cert.ACMEEnv, cert.CertPath, cert.KeyPath)
	if err != nil {
		cert.RenewError = err.Error()
		h.DB.Save(&cert)
		audit.Set(c, &audit.Entry{Action: audit.ActCertRenew, TargetType: audit.TargetCert, TargetID: idStr(cert.ID),
			Detail: "续期证书失败 " + cert.Name + "：" + err.Error(), Result: audit.ResultError})
		fail(c, http.StatusBadGateway, "acme_failed", err.Error())
		return
	}
	now := time.Now()
	cert.NotAfter, cert.Issuer, cert.LastRenewedAt, cert.RenewError = &info.NotAfter, info.Issuer, &now, ""
	if !saveOr500(c, h.DB, &cert) {
		return
	}
	if err := h.Nginx.Reload(c.Request.Context()); err != nil {
		applyErr(c, err)
		return
	}
	audit.Set(c, &audit.Entry{Action: audit.ActCertRenew, TargetType: audit.TargetCert, TargetID: idStr(cert.ID),
		Detail: "续期证书成功 " + cert.Name + "，有效期至 " + info.NotAfter.Format("2006-01-02")})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteCert removes a certificate; blocked if any site references it.
func (h *Handler) DeleteCert(c *gin.Context) {
	var cert models.Certificate
	if err := h.DB.First(&cert, c.Param("id")).Error; err != nil {
		fail(c, http.StatusNotFound, "not_found", "证书不存在")
		return
	}
	// Check usage and delete atomically: with the single SQLite connection the
	// transaction serializes against a concurrent BindSiteCert, preventing a
	// dangling cert_id / orphaned files.
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var inUse int64
		if err := tx.Model(&models.Site{}).Where("cert_id = ?", cert.ID).Count(&inUse).Error; err != nil {
			return err
		}
		if inUse > 0 {
			return errCertInUse
		}
		return tx.Delete(&cert).Error
	})
	if errors.Is(err, errCertInUse) {
		fail(c, http.StatusConflict, "cert_in_use", "该证书正被站点使用，请先在相关站点解绑后再删除")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "internal", "删除证书失败")
		return
	}
	_ = os.RemoveAll(ssl.CertDir(h.Cfg.CertsDir, cert.ID))
	audit.Set(c, &audit.Entry{Action: audit.ActCertDelete, TargetType: audit.TargetCert, TargetID: idStr(cert.ID),
		Detail: "删除证书 " + cert.Name})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
