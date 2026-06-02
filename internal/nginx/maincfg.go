package nginx

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed templates/nginx.conf.default
var defaultMainConf []byte

// MainConf is the panel-managed top-level nginx config nginx runs with (nginx -c).
// It is NOT user-editable; the panel only seeds and reads it.
func (p Paths) MainConf() string { return filepath.Join(p.ConfRoot, "nginx.conf") }

// SeedMainConfig writes the default main config if none exists yet.
func (s *Service) SeedMainConfig() error {
	path := s.Paths.MainConf()
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, defaultMainConf, 0o644)
}

// GetMainConfig returns the current main config (read-only, for diagnostics).
func (s *Service) GetMainConfig() (string, error) {
	b, err := os.ReadFile(s.Paths.MainConf())
	if err != nil {
		if os.IsNotExist(err) {
			return string(defaultMainConf), nil
		}
		return "", err
	}
	return string(b), nil
}
