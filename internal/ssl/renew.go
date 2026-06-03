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

// RunOnce performs a single renewal sweep over all enabled ACME sites.
func (s *Scheduler) RunOnce(ctx context.Context) {
	var sites []models.Site
	if err := s.DB.Where("ssl_mode = ? AND enabled = ?", models.SSLACME, true).Find(&sites).Error; err != nil {
		log.Printf("[renew] query sites: %v", err)
		return
	}
	for i := range sites {
		site := &sites[i]
		if site.CertNotAfter != nil && time.Until(*site.CertNotAfter) > renewThreshold {
			continue
		}
		s.renewOne(ctx, site)
	}
}

func (s *Scheduler) renewOne(ctx context.Context, site *models.Site) {
	target := fmt.Sprintf("%d", site.ID)
	info, err := s.Mgr.Issue(ctx, site.ServerNames, site.ACMEEmail, site.ACMEEnv, site.ID)
	if err != nil {
		site.RenewError = err.Error()
		s.DB.Save(site)
		_ = s.Rec.System(audit.ActSSLACMERenew, audit.TargetCert, target,
			fmt.Sprintf("自动续期失败 %v：%s", site.ServerNames, err.Error()), audit.ResultError)
		log.Printf("[renew] site %d failed: %v", site.ID, err)
		return
	}
	now := time.Now()
	site.CertPath, site.KeyPath = info.CertPath, info.KeyPath
	site.CertNotAfter = &info.NotAfter
	site.LastRenewedAt = &now

	// Apply before declaring success; record the failure if reload fails.
	if err := s.Nginx.ApplySite(ctx, site, true); err != nil {
		site.RenewError = "续期后 nginx 重载失败：" + err.Error()
		s.DB.Save(site)
		_ = s.Rec.System(audit.ActSSLACMERenew, audit.TargetCert, target,
			fmt.Sprintf("续期后重载失败：%s", err.Error()), audit.ResultError)
		return
	}
	site.RenewError = ""
	s.DB.Save(site)
	_ = s.Rec.System(audit.ActSSLACMERenew, audit.TargetCert, target,
		fmt.Sprintf("自动续期成功 %v，有效期至 %s", site.ServerNames, info.NotAfter.Format("2006-01-02")), audit.ResultOK)
}
