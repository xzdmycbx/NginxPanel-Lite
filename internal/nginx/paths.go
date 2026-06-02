package nginx

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Paths is the single source of truth for the shared-volume layout (1Panel-style
// nested includes): nginx.conf -> sites/site-<id>/site.conf -> locations/*.conf.
type Paths struct {
	ConfRoot    string // e.g. /etc/nginx-panel
	CertsDir    string // e.g. /etc/nginx-panel/certs
	AcmeWebroot string // e.g. /var/www/acme-webroot
	LogRoot     string // e.g. /etc/nginx-panel/logs (per-domain logs, panel-readable)
}

func (p Paths) SitesRoot() string { return filepath.Join(p.ConfRoot, "sites") }
func (p Paths) Staging() string   { return filepath.Join(p.ConfRoot, ".staging") }
func (p Paths) Backups() string   { return filepath.Join(p.ConfRoot, "backups") }

// Slug is the stable per-site identifier used for file/dir names.
func Slug(id uint) string { return fmt.Sprintf("site-%d", id) }

func tsStr(ts time.Time) string { return ts.UTC().Format("20060102T150405Z") }

// --- editable site files ---
func (p Paths) SiteDir(id uint) string      { return filepath.Join(p.SitesRoot(), Slug(id)) }
func (p Paths) SiteConfPath(id uint) string { return filepath.Join(p.SiteDir(id), "site.conf") }
func (p Paths) LocationsDir(id uint) string { return filepath.Join(p.SiteDir(id), "locations") }
func (p Paths) LocationPath(id uint, slug string) string {
	return filepath.Join(p.LocationsDir(id), slug+".conf")
}

// LocationsGlob is the include target written into site.conf (forward slashes for nginx).
func (p Paths) LocationsGlob(id uint) string {
	return filepath.ToSlash(p.LocationsDir(id)) + "/*.conf"
}

// --- apply pipeline scratch dirs ---
func (p Paths) StagingSiteDir(id uint) string { return filepath.Join(p.Staging(), Slug(id)) }
func (p Paths) StagingAside(id uint, ts time.Time) string {
	// NOTE: under .staging (NOT under sites/) so the include glob never matches it.
	return filepath.Join(p.Staging(), Slug(id)+".old."+tsStr(ts))
}
func (p Paths) BackupRoot(id uint) string { return filepath.Join(p.Backups(), Slug(id)) }
func (p Paths) BackupSiteDir(id uint, ts time.Time) string {
	return filepath.Join(p.BackupRoot(id), tsStr(ts))
}

// --- per-domain logs ---
func (p Paths) LogDir(id uint) string    { return filepath.Join(p.LogRoot, Slug(id)) }
func (p Paths) AccessLog(id uint) string { return filepath.Join(p.LogDir(id), "access.log") }
func (p Paths) ErrorLog(id uint) string  { return filepath.Join(p.LogDir(id), "error.log") }

// --- certs ---
func (p Paths) CertDir(id uint) string { return filepath.Join(p.CertsDir, Slug(id)) }
func (p Paths) CertFiles(id uint) (cert, key string) {
	d := p.CertDir(id)
	return filepath.Join(d, "fullchain.pem"), filepath.Join(d, "privkey.pem")
}

// EnsureDirs creates all managed directories.
func (p Paths) EnsureDirs() error {
	for _, d := range []string{p.SitesRoot(), p.Staging(), p.Backups(), p.CertsDir, p.AcmeWebroot, p.LogRoot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
