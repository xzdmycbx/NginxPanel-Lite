// Package database opens the SQLite database (pure-Go driver, no CGO),
// runs migrations, and seeds initial settings.
package database

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
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

// Migrate runs AutoMigrate over all models.
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(models.All()...)
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
