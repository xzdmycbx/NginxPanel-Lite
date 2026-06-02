package nginx

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
)

// ApplyError carries the nginx validation/reload failure detail back to the user.
type ApplyError struct {
	Stage       string // "validate" | "reload"
	NginxOutput string
}

func (e *ApplyError) Error() string {
	return fmt.Sprintf("nginx %s 失败: %s", e.Stage, e.NginxOutput)
}

// Service generates per-site config trees and applies them to nginx with rollback.
type Service struct {
	Paths Paths
	Ctl   *Controller
	mu    sync.Mutex
}

func NewService(p Paths, ctl *Controller) *Service {
	return &Service{Paths: p, Ctl: ctl}
}

// Render returns the rendered fileset for a site.
func (s *Service) Render(site *models.Site, enableTLS bool) (SiteFiles, error) {
	return Render(site, s.Paths, enableTLS)
}

// Reload validates and reloads nginx (no file change).
func (s *Service) Reload(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if out, ok, err := s.Ctl.TestConfig(ctx); err != nil {
		return fmt.Errorf("nginx -t 无法执行: %w", err)
	} else if !ok {
		return &ApplyError{Stage: "validate", NginxOutput: out}
	}
	return s.Ctl.Reload(ctx)
}

// ApplySite renders and applies a site's whole config tree.
func (s *Service) ApplySite(ctx context.Context, site *models.Site, enableTLS bool) error {
	files, err := Render(site, s.Paths, enableTLS)
	if err != nil {
		return err
	}
	return s.applyStagedSite(ctx, site.ID, files)
}

// EnsureChallengeServer provisions a plain :80 server (for ACME HTTP-01).
func (s *Service) EnsureChallengeServer(ctx context.Context, site *models.Site) error {
	return s.ApplySite(ctx, site, false)
}

// applyStagedSite swaps the whole site directory atomically (same volume), then
// validates + reloads, restoring the entire previous directory on any failure.
func (s *Service) applyStagedSite(ctx context.Context, id uint, files SiteFiles) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyStagedSiteLocked(ctx, id, files)
}

// applyStagedSiteLocked is applyStagedSite without acquiring the mutex (the
// caller must already hold s.mu). Lets read-modify-apply flows (SaveSiteFile)
// run atomically against concurrent applies/removes.
func (s *Service) applyStagedSiteLocked(ctx context.Context, id uint, files SiteFiles) error {
	if err := s.Paths.EnsureDirs(); err != nil {
		return err
	}
	_ = os.MkdirAll(s.Paths.LogDir(id), 0o755) // per-domain log target must exist for nginx

	siteDir := s.Paths.SiteDir(id)
	staging := s.Paths.StagingSiteDir(id)
	ts := time.Now()

	_ = os.RemoveAll(staging)
	defer os.RemoveAll(staging) // cleanup any leftover staging on failure paths
	if err := writeSiteTree(staging, files); err != nil {
		return err
	}

	hadPrev := dirExists(siteDir)
	if hadPrev {
		if err := copyTree(siteDir, s.Paths.BackupSiteDir(id, ts)); err != nil {
			return err
		}
	}

	aside := s.Paths.StagingAside(id, ts) // NOT under sites/ -> include glob never matches it
	if hadPrev {
		if err := os.Rename(siteDir, aside); err != nil {
			return err
		}
	}
	if err := os.Rename(staging, siteDir); err != nil {
		if hadPrev {
			_ = os.Rename(aside, siteDir)
		}
		return err
	}
	restore := func() {
		_ = os.RemoveAll(siteDir)
		if hadPrev {
			_ = os.Rename(aside, siteDir)
		}
	}

	out, ok, err := s.Ctl.TestConfig(ctx)
	if err != nil {
		restore()
		return fmt.Errorf("nginx -t 无法执行: %w", err)
	}
	if !ok {
		restore()
		return &ApplyError{Stage: "validate", NginxOutput: out}
	}
	if err := s.Ctl.Reload(ctx); err != nil {
		restore()
		if ae, isApply := err.(*ApplyError); isApply {
			return ae
		}
		return &ApplyError{Stage: "reload", NginxOutput: err.Error()}
	}
	_ = os.RemoveAll(aside)
	s.pruneBackups(id, 10)
	return nil
}

// RemoveSite deletes a site's whole directory through the same gate, restoring
// it on validation/reload failure.
func (s *Service) RemoveSite(ctx context.Context, id uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	siteDir := s.Paths.SiteDir(id)
	if !dirExists(siteDir) {
		return nil
	}
	ts := time.Now()
	if err := copyTree(siteDir, s.Paths.BackupSiteDir(id, ts)); err != nil {
		return err
	}
	aside := s.Paths.StagingAside(id, ts)
	if err := os.Rename(siteDir, aside); err != nil {
		return err
	}
	restore := func() { _ = os.Rename(aside, siteDir) }

	out, ok, err := s.Ctl.TestConfig(ctx)
	if err != nil {
		restore()
		return fmt.Errorf("nginx -t 无法执行: %w", err)
	}
	if !ok {
		restore()
		return &ApplyError{Stage: "validate", NginxOutput: out}
	}
	if err := s.Ctl.Reload(ctx); err != nil {
		restore()
		return &ApplyError{Stage: "reload", NginxOutput: err.Error()}
	}
	_ = os.RemoveAll(aside)
	return nil
}

// ReconcileSites materializes any enabled site MISSING from disk (migration /
// bootstrap) and removes disabled sites + the legacy conf.d. It deliberately
// does NOT overwrite existing site trees, so admin raw edits survive restarts
// and a previously-valid model is only re-rendered when its tree is absent.
func (s *Service) ReconcileSites(sites []models.Site) {
	_ = s.Paths.EnsureDirs()
	_ = os.RemoveAll(filepath.Join(s.Paths.ConfRoot, "conf.d")) // legacy single-file layout
	for i := range sites {
		site := &sites[i]
		if !site.Enabled {
			_ = os.RemoveAll(s.Paths.SiteDir(site.ID))
			continue
		}
		_ = os.MkdirAll(s.Paths.LogDir(site.ID), 0o755)
		if fileExists(s.Paths.SiteConfPath(site.ID)) {
			continue // already on disk — preserve it (incl. manual edits)
		}
		enableTLS := site.SSLMode != models.SSLNone && site.CertPath != "" && fileExists(site.CertPath)
		files, err := Render(site, s.Paths, enableTLS)
		if err != nil {
			log.Printf("[reconcile] site %d render: %v", site.ID, err)
			continue
		}
		if err := writeSiteTree(s.Paths.SiteDir(site.ID), files); err != nil {
			log.Printf("[reconcile] site %d write: %v", site.ID, err)
		}
	}
}

// --- editable files ---

// SiteFileInfo describes one editable file of a site.
type SiteFileInfo struct {
	Key   string `json:"key"`   // "site" or "loc:<slug>"
	Label string `json:"label"` // human label
}

// ListSiteFiles returns the editable files of a site (domain main + locations).
func (s *Service) ListSiteFiles(id uint) []SiteFileInfo {
	out := []SiteFileInfo{{Key: "site", Label: "域名主文件 (site.conf)"}}
	entries, err := os.ReadDir(s.Paths.LocationsDir(id))
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".conf")
		label := "location " + slug
		if b, err := os.ReadFile(filepath.Join(s.Paths.LocationsDir(id), e.Name())); err == nil {
			if p := parseLocationPath(b); p != "" {
				label = "location " + p
			}
		}
		out = append(out, SiteFileInfo{Key: "loc:" + slug, Label: label})
	}
	return out
}

// GetSiteFile reads one editable file's content.
func (s *Service) GetSiteFile(id uint, key string) (string, error) {
	path, err := s.fileForKey(id, key)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

// SaveSiteFile replaces one file in the current tree and re-applies the whole
// site through the validate/reload/rollback gate. The read-modify-apply runs
// under the service mutex so it never observes a directory mid-swap.
func (s *Service) SaveSiteFile(ctx context.Context, id uint, key string, content []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, err := readSiteTreeFromDir(s.Paths.SiteDir(id))
	if err != nil {
		return err
	}
	if key == "site" {
		files.SiteConf = content
	} else if strings.HasPrefix(key, "loc:") {
		slug := key[len("loc:"):]
		if !safeSlug(slug) {
			return fmt.Errorf("无效的文件标识")
		}
		found := false
		for i := range files.Locations {
			if files.Locations[i].Slug == slug {
				files.Locations[i].Content = content
				found = true
			}
		}
		if !found {
			return fmt.Errorf("location 文件不存在")
		}
	} else {
		return fmt.Errorf("无效的文件标识")
	}
	return s.applyStagedSiteLocked(ctx, id, files)
}

func (s *Service) fileForKey(id uint, key string) (string, error) {
	if key == "site" {
		return s.Paths.SiteConfPath(id), nil
	}
	if strings.HasPrefix(key, "loc:") {
		slug := key[len("loc:"):]
		if !safeSlug(slug) {
			return "", fmt.Errorf("无效的文件标识")
		}
		return s.Paths.LocationPath(id, slug), nil
	}
	return "", fmt.Errorf("无效的文件标识")
}

func (s *Service) readSiteTree(id uint) (SiteFiles, error) {
	return readSiteTreeFromDir(s.Paths.SiteDir(id))
}

// --- backups ---

type Backup struct {
	Timestamp string `json:"timestamp"`
	CreatedAt string `json:"createdAt"`
}

func (s *Service) ListBackups(id uint) ([]Backup, error) {
	entries, err := os.ReadDir(s.Paths.BackupRoot(id))
	if err != nil {
		if os.IsNotExist(err) {
			return []Backup{}, nil
		}
		return nil, err
	}
	var out []Backup
	for _, e := range entries {
		if !e.IsDir() || !isPlainTimestamp(e.Name()) {
			continue
		}
		readable := e.Name()
		if t, err := time.Parse("20060102T150405Z", e.Name()); err == nil {
			readable = t.Local().Format("2006-01-02 15:04:05")
		}
		out = append(out, Backup{Timestamp: e.Name(), CreatedAt: readable})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp > out[j].Timestamp })
	return out, nil
}

func (s *Service) RestoreBackup(ctx context.Context, id uint, ts string) error {
	if !isPlainTimestamp(ts) {
		return fmt.Errorf("无效的备份标识")
	}
	files, err := readSiteTreeFromDir(filepath.Join(s.Paths.BackupRoot(id), ts))
	if err != nil {
		return err
	}
	return s.applyStagedSite(ctx, id, files)
}

func (s *Service) pruneBackups(id uint, keep int) {
	backups, err := s.ListBackups(id)
	if err != nil || len(backups) <= keep {
		return
	}
	for _, b := range backups[keep:] {
		_ = os.RemoveAll(filepath.Join(s.Paths.BackupRoot(id), b.Timestamp))
	}
}

// --- logs ---

// TailSiteLog returns the tail of a site's access or error log.
func (s *Service) TailSiteLog(id uint, kind string, lines int) ([]string, error) {
	path := s.Paths.AccessLog(id)
	if kind == "error" {
		path = s.Paths.ErrorLog(id)
	}
	return tailFile(path, lines, defaultTailBytes)
}

// ClearSiteLog truncates a site log in place (nginx keeps its fd, no reload).
func (s *Service) ClearSiteLog(id uint, kind string) error {
	path := s.Paths.AccessLog(id)
	if kind == "error" {
		path = s.Paths.ErrorLog(id)
	}
	if err := os.Truncate(path, 0); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// --- helpers ---

func writeSiteTree(dir string, files SiteFiles) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, "locations"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "site.conf"), files.SiteConf, 0o644); err != nil {
		return err
	}
	for _, l := range files.Locations {
		if err := os.WriteFile(filepath.Join(dir, "locations", l.Slug+".conf"), l.Content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func readSiteTreeFromDir(dir string) (SiteFiles, error) {
	sc, err := os.ReadFile(filepath.Join(dir, "site.conf"))
	if err != nil && !os.IsNotExist(err) {
		return SiteFiles{}, err
	}
	var locs []NamedFile
	entries, _ := os.ReadDir(filepath.Join(dir, "locations"))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, "locations", e.Name()))
		if err != nil {
			return SiteFiles{}, err
		}
		locs = append(locs, NamedFile{Slug: strings.TrimSuffix(e.Name(), ".conf"), Content: b})
	}
	sort.Slice(locs, func(i, j int) bool { return locs[i].Slug < locs[j].Slug })
	return SiteFiles{SiteConf: sc, Locations: locs}, nil
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func readIfExists(path string) ([]byte, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return b, true
}

func safeSlug(slug string) bool {
	return slug != "" && !strings.ContainsAny(slug, "/\\") && !strings.Contains(slug, "..")
}

func isPlainTimestamp(ts string) bool {
	_, err := time.Parse("20060102T150405Z", ts)
	return err == nil
}

// parseLocationPath extracts the path from a `location <path> {` block.
func parseLocationPath(content []byte) string {
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "location ") {
			rest := strings.TrimSpace(strings.TrimPrefix(line, "location "))
			rest = strings.TrimSuffix(strings.TrimSpace(strings.TrimSuffix(rest, "{")), " ")
			return strings.TrimSpace(rest)
		}
	}
	return ""
}
