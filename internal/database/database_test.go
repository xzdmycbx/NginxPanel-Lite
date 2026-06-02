package database

import (
	"path/filepath"
	"testing"

	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
)

// TestSeedSettingsIdempotent reproduces the restart-after-setup crash: once
// setup flips first_run to "false", a second SeedSettings must not error or
// duplicate the row.
func TestSeedSettingsIdempotent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s, _ := db.DB(); s.Close() }()

	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := SeedSettings(db); err != nil {
		t.Fatalf("first seed: %v", err)
	}

	// Simulate setup flipping the flag.
	db.Model(&models.Setting{}).Where("key = ?", SettingFirstRun).Update("value", "false")

	// Second seed (restart) must be a no-op, not a unique-constraint crash.
	if err := SeedSettings(db); err != nil {
		t.Fatalf("second seed crashed: %v", err)
	}

	var s models.Setting
	db.First(&s, "key = ?", SettingFirstRun)
	if s.Value != "false" {
		t.Fatalf("seed overwrote the flag; want false, got %q", s.Value)
	}
	var n int64
	db.Model(&models.Setting{}).Where("key = ?", SettingFirstRun).Count(&n)
	if n != 1 {
		t.Fatalf("expected exactly 1 settings row, got %d", n)
	}
}
