package ssl

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/xzdmycbx/nginxpanel-lite/internal/audit"
	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
	"github.com/xzdmycbx/nginxpanel-lite/internal/nginx"
)

const renewThreshold = 30 * 24 * time.Hour

// Scheduler renews ACME certificates that are close to expiry.
type Scheduler struct {
	DB    *gorm.DB
	Mgr   *Manager
	Nginx *nginx.Service
	Rec   *audit.Recorder
	cron  *cron.Cron
}

func NewScheduler(db *gorm.DB, mgr *Manager, ng *nginx.Service, rec *audit.Recorder) *Scheduler {
	return &Scheduler{DB: db, Mgr: mgr, Nginx: ng, Rec: rec}
}

// Start runs the daily renewal sweep at 03:17.
func (s *Scheduler) Start() {
	c := cron.New()
	_, _ = c.AddFunc("17 3 * * *", func() { s.RunOnce(context.Background()) })
	c.Start()
	s.cron = c
}

func (s *Scheduler) Stop() {
	if s.cron != nil {
		s.cron.Stop()
	}
}

// RunOnce renews ACME certificates near expiry, then reloads nginx once if any
// certificate's files changed (so sites using them pick up the new content).
func (s *Scheduler) RunOnce(ctx context.Context) {
	var certs []models.Certificate
	if err := s.DB.Where("source = ?", models.CertACME).Find(&certs).Error; err != nil {
		log.Printf("[renew] query certs: %v", err)
		return
	}
	changed := false
	for i := range certs {
		cert := &certs[i]
		if cert.NotAfter != nil && time.Until(*cert.NotAfter) > renewThreshold {
			continue
		}
		if s.renewOne(ctx, cert) {
			changed = true
		}
	}
	if changed {
		if err := s.Nginx.Reload(ctx); err != nil {
			log.Printf("[renew] nginx reload: %v", err)
		}
	}
}

// renewOne re-issues one certificate to its existing paths. Returns true if it
// succeeded (so the caller knows to reload nginx).
func (s *Scheduler) renewOne(ctx context.Context, cert *models.Certificate) bool {
	target := fmt.Sprintf("%d", cert.ID)
	info, err := s.Mgr.Issue(ctx, cert.Domains, cert.ACMEEmail, cert.ACMEEnv, cert.CertPath, cert.KeyPath)
	if err != nil {
		cert.RenewError = err.Error()
		s.DB.Save(cert)
		_ = s.Rec.System(audit.ActCertRenew, audit.TargetCert, target,
			fmt.Sprintf("自动续期失败 %s：%s", cert.Name, err.Error()), audit.ResultError)
		log.Printf("[renew] cert %d failed: %v", cert.ID, err)
		return false
	}
	now := time.Now()
	cert.NotAfter, cert.Issuer, cert.LastRenewedAt, cert.RenewError = &info.NotAfter, info.Issuer, &now, ""
	s.DB.Save(cert)
	_ = s.Rec.System(audit.ActCertRenew, audit.TargetCert, target,
		fmt.Sprintf("自动续期成功 %s，有效期至 %s", cert.Name, info.NotAfter.Format("2006-01-02")), audit.ResultOK)
	return true
}
