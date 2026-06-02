package nginx

import (
	"strings"
	"testing"

	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
)

func testPaths() Paths {
	return Paths{
		ConfRoot:    "/etc/nginx-panel",
		CertsDir:    "/etc/nginx-panel/certs",
		AcmeWebroot: "/var/www/acme-webroot",
		LogRoot:     "/etc/nginx-panel/logs",
	}
}

func locContent(files SiteFiles, idx int) string {
	if idx >= len(files.Locations) {
		return ""
	}
	return string(files.Locations[idx].Content)
}

func TestRenderPlainHTTP(t *testing.T) {
	site := &models.Site{ID: 1, ServerNames: []string{"a.example.com"}, UpstreamTargets: []string{"app:3000"}}
	files, err := Render(site, testPaths(), false)
	if err != nil {
		t.Fatal(err)
	}
	sc := string(files.SiteConf)
	for _, want := range []string{
		"listen 80;", "server_name a.example.com;", "/.well-known/acme-challenge/",
		"include /etc/nginx-panel/sites/site-1/locations/*.conf;",
		"access_log /etc/nginx-panel/logs/site-1/access.log main;",
		"error_log  /etc/nginx-panel/logs/site-1/error.log warn;",
	} {
		if !strings.Contains(sc, want) {
			t.Errorf("site.conf missing %q in:\n%s", want, sc)
		}
	}
	if strings.Contains(sc, "listen 443") {
		t.Errorf("did not expect TLS block:\n%s", sc)
	}
	loc := locContent(files, 0)
	for _, want := range []string{"location / {", "proxy_pass http://app:3000;"} {
		if !strings.Contains(loc, want) {
			t.Errorf("location file missing %q in:\n%s", want, loc)
		}
	}
}

func TestRenderTLSWithRedirect(t *testing.T) {
	site := &models.Site{
		ID: 2, ServerNames: []string{"b.example.com"}, UpstreamTargets: []string{"http://app:8080"},
		SSLMode: models.SSLManual, ForceHTTPSRedirect: true,
		CertPath: "/etc/nginx-panel/certs/site-2/fullchain.pem", KeyPath: "/etc/nginx-panel/certs/site-2/privkey.pem",
	}
	files, err := Render(site, testPaths(), true)
	if err != nil {
		t.Fatal(err)
	}
	sc := string(files.SiteConf)
	for _, want := range []string{
		"listen 443 ssl;", "return 301 https://$host$request_uri;",
		"ssl_certificate     /etc/nginx-panel/certs/site-2/fullchain.pem;",
		"include /etc/nginx-panel/sites/site-2/locations/*.conf;",
	} {
		if !strings.Contains(sc, want) {
			t.Errorf("site.conf missing %q in:\n%s", want, sc)
		}
	}
}

func TestRenderWebsocketAndMultiUpstream(t *testing.T) {
	site := &models.Site{
		ID: 3, ServerNames: []string{"c.example.com"},
		UpstreamTargets: []string{"10.0.0.1:80", "10.0.0.2:80"}, WebsocketUpgrade: true,
	}
	files, err := Render(site, testPaths(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files.SiteConf), "upstream site_3_loc0_backend {") {
		t.Errorf("expected upstream block in site.conf:\n%s", files.SiteConf)
	}
	loc := locContent(files, 0)
	for _, want := range []string{"proxy_pass http://site_3_loc0_backend;", "proxy_set_header Upgrade $http_upgrade;"} {
		if !strings.Contains(loc, want) {
			t.Errorf("location file missing %q in:\n%s", want, loc)
		}
	}
}

func TestRenderMultiLocationWithCache(t *testing.T) {
	site := &models.Site{
		ID: 7, ServerNames: []string{"app.example.com"},
		Locations: []models.ProxyLocation{
			{Path: "/", UpstreamTargets: []string{"http://web:3000"}},
			{Path: "/api", UpstreamTargets: []string{"http://api:8080"}, CacheEnabled: true, WebsocketUpgrade: true},
			{Path: "/static", UpstreamTargets: []string{"cdn:80"}, ExtraConfig: "expires 7d;"},
		},
	}
	files, err := Render(site, testPaths(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files.Locations) != 3 {
		t.Fatalf("expected 3 location files, got %d", len(files.Locations))
	}
	if !strings.Contains(locContent(files, 0), "proxy_pass http://web:3000;") {
		t.Errorf("loc0 wrong:\n%s", locContent(files, 0))
	}
	api := locContent(files, 1)
	for _, want := range []string{"location /api {", "proxy_pass http://api:8080;", "proxy_cache npl_cache;", "proxy_set_header Upgrade $http_upgrade;"} {
		if !strings.Contains(api, want) {
			t.Errorf("loc /api missing %q in:\n%s", want, api)
		}
	}
	if !strings.Contains(locContent(files, 2), "expires 7d;") {
		t.Errorf("loc /static missing extra config:\n%s", locContent(files, 2))
	}
	// cache only on /api
	if strings.Contains(locContent(files, 0), "proxy_cache npl_cache;") {
		t.Errorf("cache should not be on /")
	}
}

func TestRenderRequiresDomainAndUpstream(t *testing.T) {
	if _, err := Render(&models.Site{ID: 4, UpstreamTargets: []string{"x:1"}}, testPaths(), false); err == nil {
		t.Error("expected error for missing domain")
	}
	if _, err := Render(&models.Site{ID: 5, ServerNames: []string{"x.com"}}, testPaths(), false); err == nil {
		t.Error("expected error for missing upstream")
	}
}

func TestLocationSlug(t *testing.T) {
	cases := map[string]string{"/": "00-root", "/api": "00-api", "= /x": "00-x"}
	for path, want := range cases {
		if got := locationSlug(path, 0); got != want {
			t.Errorf("locationSlug(%q) = %q, want %q", path, got, want)
		}
	}
}
