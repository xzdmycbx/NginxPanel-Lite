// Package database opens the SQLite database (pure-Go driver, no CGO),
// runs migrations, and seeds initial settings.
package database

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
	"github.com/xzdmycbx/nginxpanel-lite/internal/ssl"
)

const SettingFirstRun = "first_run"

// Open opens the SQLite database at path with WAL + busy_timeout + foreign_keys
// pragmas suited to the panel's concurrent web requests.
func Open(path string) (*gorm.DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)", path)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// SQLite is a single-writer; keep the pool small to avoid lock contention.
	sqlDB.SetMaxOpenConns(1)
	return db, nil
}

// Migrate runs AutoMigrate over all models, then backfills the system admin and
// converts legacy per-site certificates for databases created before those
// fields existed.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(models.All()...); err != nil {
		return err
	}
	if err := backfillSystemAdmin(db); err != nil {
		return err
	}
	return backfillCerts(db)
}

// backfillCerts converts legacy per-site certificates (older schema) into named
// global Certificate records and binds the site to them. Best-effort: a fresh DB
// has no legacy ssl_mode column and is skipped.
func backfillCerts(db *gorm.DB) error {
	type legacy struct {
		ID        uint
		SSLMode   string
		CertPath  string
		KeyPath   string
		ACMEEmail string
		ACMEEnv   string
	}
	var rows []legacy
	if err := db.Table("sites").
		Where("cert_id IS NULL AND ssl_mode IS NOT NULL AND ssl_mode != 'none' AND cert_path != ''").
		Find(&rows).Error; err != nil {
		if strings.Contains(err.Error(), "no such column") {
			return nil // fresh DB — legacy ssl columns never existed, nothing to migrate
		}
		return err
	}
	for _, r := range rows {
		b, err := os.ReadFile(r.CertPath)
		if err != nil {
			continue // cert file gone — leave the site without TLS
		}
		info, err := ssl.ParseCertInfo(b)
		if err != nil {
			continue
		}
		source := models.CertManual
		if r.SSLMode == string(models.CertACME) {
			source = models.CertACME
		}
		cert := models.Certificate{
			Name: fmt.Sprintf("站点%d-旧证书", r.ID), Source: source, Domains: info.Domains,
			CertPath: r.CertPath, KeyPath: r.KeyPath, NotAfter: &info.NotAfter, Issuer: info.Issuer,
			ACMEEmail: r.ACMEEmail, ACMEEnv: r.ACMEEnv,
		}
		if err := db.Create(&cert).Error; err != nil {
			continue
		}
		db.Table("sites").Where("id = ?", r.ID).Update("cert_id", cert.ID)
	}
	return nil
}

// backfillSystemAdmin promotes the earliest admin to system admin if none is
// marked yet (older DBs). Fresh installs have no admin here — Setup marks it.
func backfillSystemAdmin(db *gorm.DB) error {
	var marked int64
	if err := db.Model(&models.User{}).Where("system_admin = ?", true).Count(&marked).Error; err != nil {
		return err
	}
	if marked > 0 {
		return nil
	}
	var first models.User
	if err := db.Where("role = ?", models.RoleAdmin).Order("id asc").First(&first).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // fresh install, no admin yet
		}
		return err
	}
	return db.Model(&first).Update("system_admin", true).Error
}

// SeedSettings ensures the first_run flag exists (defaults to "true").
// The lookup is by key ONLY; Attrs supplies the default value just for the
// create path, so restarting after setup (first_run="false") is idempotent and
// never re-inserts the row.
func SeedSettings(db *gorm.DB) error {
	return db.Where(models.Setting{Key: SettingFirstRun}).
		Attrs(models.Setting{Value: "true"}).
		FirstOrCreate(&models.Setting{}).Error
}
