package nginx

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
)

func testService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	p := Paths{
		ConfRoot:    filepath.Join(dir, "np"),
		CertsDir:    filepath.Join(dir, "np", "certs"),
		AcmeWebroot: filepath.Join(dir, "acme"),
		LogRoot:     filepath.Join(dir, "np", "logs"),
	}
	ctl, err := NewController("", "test", p.MainConf(), true) // dry-run
	if err != nil {
		t.Fatal(err)
	}
	return NewService(p, ctl)
}

func demoSite(id uint, domain string) *models.Site {
	return &models.Site{
		ID: id, Enabled: true, ServerNames: []string{domain},
		Locations: []models.ProxyLocation{{Path: "/", UpstreamTargets: []string{"app:3000"}}},
	}
}

func TestApplyListAndEditFile(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if err := s.ApplySite(ctx, demoSite(1, "a.com"), false); err != nil {
		t.Fatal(err)
	}

	main, err := s.GetSiteFile(1, "site")
	if err != nil || !strings.Contains(main, "server_name a.com;") {
		t.Fatalf("site.conf wrong: %v / %s", err, main)
	}

	files := s.ListSiteFiles(1)
	if len(files) != 2 {
		t.Fatalf("expected site + 1 location, got %d (%v)", len(files), files)
	}
	var locKey string
	for _, f := range files {
		if strings.HasPrefix(f.Key, "loc:") {
			locKey = f.Key
		}
	}
	// raw-edit the location file
	if err := s.SaveSiteFile(ctx, 1, locKey, []byte("location / {\n    proxy_pass http://changed:9999;\n}\n")); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetSiteFile(1, locKey)
	if !strings.Contains(got, "changed:9999") {
		t.Fatalf("edit not persisted: %s", got)
	}

	// path-traversal guard on the file key
	if _, err := s.GetSiteFile(1, "loc:../../etc/passwd"); err == nil {
		t.Error("expected error for traversal key")
	}
}

func TestReconcilePreservesRawEdit(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	site := demoSite(2, "b.com")
	if err := s.ApplySite(ctx, site, false); err != nil {
		t.Fatal(err)
	}
	raw := "# raw edited\nserver {\n    listen 80;\n    server_name b.com;\n    location / { proxy_pass http://x:1; }\n}\n"
	if err := s.SaveSiteFile(ctx, 2, "site", []byte(raw)); err != nil {
		t.Fatal(err)
	}
	// reconcile must NOT overwrite the existing (raw-edited) tree
	s.ReconcileSites([]models.Site{*site})
	content, _ := s.GetSiteFile(2, "site")
	if !strings.Contains(content, "# raw edited") {
		t.Fatalf("reconcile overwrote the manual edit:\n%s", content)
	}
}

func TestRemoveSite(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if err := s.ApplySite(ctx, demoSite(3, "c.com"), false); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveSite(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.Paths.SiteDir(3)); !os.IsNotExist(err) {
		t.Fatal("site dir should be removed")
	}
}
