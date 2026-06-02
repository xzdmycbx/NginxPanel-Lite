package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/xzdmycbx/nginxpanel-lite/internal/app"
	"github.com/xzdmycbx/nginxpanel-lite/internal/config"
)

// TestEndToEndAuthAndSite walks the full first-run -> TOTP -> site-create -> log
// flow against the real router in nginx dry-run mode.
func TestEndToEndAuthAndSite(t *testing.T) {
	t.Setenv("PANEL_DATA_DIR", t.TempDir())
	t.Setenv("PANEL_NGINX_DRYRUN", "true")
	t.Setenv("PANEL_COOKIE_SECURE", "false")
	t.Setenv("PANEL_JWT_SECRET", "test-secret-for-end-to-end-testing-1234")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config load: %v", err)
	}
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app new: %v", err)
	}
	defer application.Close()
	srv := httptest.NewServer(application.Server.Handler)
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	post := func(path string, body any) map[string]any {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		resp, err := client.Post(srv.URL+path, "application/json", &buf)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		out["_status"] = float64(resp.StatusCode)
		return out
	}
	get := func(path string) (int, map[string]any) {
		t.Helper()
		resp, err := client.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	// 1. me -> needsSetup
	_, me := get("/api/auth/me")
	if me["needsSetup"] != true {
		t.Fatalf("expected needsSetup=true, got %v", me)
	}

	// 2. setup first admin -> needs_totp_enroll
	r := post("/api/setup", map[string]string{"username": "admin", "password": "Admin#1234"})
	if r["next"] != "needs_totp_enroll" {
		t.Fatalf("setup next=%v (status %v)", r["next"], r["_status"])
	}

	// 3. enroll TOTP
	enroll := post("/api/auth/totp/enroll", nil)
	secret, _ := enroll["secret"].(string)
	if secret == "" {
		t.Fatalf("no secret returned: %v", enroll)
	}

	// 4. activate with a generated code
	code, _ := totp.GenerateCode(secret, time.Now())
	act := post("/api/auth/totp/activate", map[string]string{"code": code})
	if act["next"] != "done" {
		t.Fatalf("activate next=%v (status %v)", act["next"], act["_status"])
	}

	// 5. me -> authenticated admin
	_, me2 := get("/api/auth/me")
	if me2["authenticated"] != true {
		t.Fatalf("expected authenticated, got %v", me2)
	}

	// 6. create a reverse-proxy site (nginx dry-run)
	site := post("/api/sites", map[string]any{
		"name":            "demo",
		"serverNames":     []string{"demo.example.com"},
		"upstreamTargets": []string{"http://app:3000"},
	})
	if site["_status"] != float64(200) {
		t.Fatalf("create site failed: %v", site)
	}

	// 7. list sites -> one
	status, list := get("/api/sites")
	if status != 200 {
		t.Fatalf("list sites status %d", status)
	}
	if items, ok := list["items"].([]any); !ok || len(items) != 1 {
		t.Fatalf("expected 1 site, got %v", list["items"])
	}

	// 8. audit log captured the create
	_, logs := get("/api/logs")
	items, _ := logs["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("expected audit log entries")
	}

	// 9. injection-style input is rejected (nginx directive breakout attempt)
	bad := post("/api/sites", map[string]any{
		"name":            "evil",
		"serverNames":     []string{"evil;}\nserver{listen 9999"},
		"upstreamTargets": []string{"http://app:3000"},
	})
	if bad["_status"] != float64(400) {
		t.Fatalf("expected 400 for malicious server_name, got %v", bad["_status"])
	}

	// 10. multi-location site with per-location cache + websocket
	ml := post("/api/sites", map[string]any{
		"name":        "multi",
		"serverNames": []string{"multi.example.com"},
		"locations": []map[string]any{
			{"path": "/", "upstreamTargets": []string{"http://web:3000"}},
			{"path": "/api", "upstreamTargets": []string{"http://api:8080"}, "cacheEnabled": true, "websocketUpgrade": true},
		},
	})
	if ml["_status"] != float64(200) {
		t.Fatalf("create multi-location site failed: %v", ml)
	}
	mlID := int(ml["id"].(float64))

	// 11. its editable config files are listed (domain main + per-location)
	_, files := get(fmt.Sprintf("/api/sites/%d/files", mlID))
	if items, _ := files["items"].([]any); len(items) < 3 {
		t.Fatalf("expected site + 2 location files, got %v", files["items"])
	}

	// 12. per-domain log tail responds (empty in dry-run, but the route works)
	st, _ := get(fmt.Sprintf("/api/sites/%d/logs?type=access", mlID))
	if st != 200 {
		t.Fatalf("site logs status %d", st)
	}
}

// TestTOTPReenrollBlocked is a regression test for the auth-bypass where a
// password-only attacker (holding a StageTOTP cookie) could overwrite an
// enrolled user's TOTP secret via /auth/totp/enroll.
func TestTOTPReenrollBlocked(t *testing.T) {
	t.Setenv("PANEL_DATA_DIR", t.TempDir())
	t.Setenv("PANEL_NGINX_DRYRUN", "true")
	t.Setenv("PANEL_COOKIE_SECURE", "false")
	t.Setenv("PANEL_JWT_SECRET", "totp-reenroll-test-secret-1234567890")

	cfg, _ := config.Load()
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app new: %v", err)
	}
	defer application.Close()
	srv := httptest.NewServer(application.Server.Handler)
	defer srv.Close()

	doPost := func(client *http.Client, path string, body any) (int, map[string]any) {
		t.Helper()
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		resp, err := client.Post(srv.URL+path, "application/json", &buf)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	// Legit user sets up and enrolls TOTP (now TOTPEnabled=true).
	jar1, _ := cookiejar.New(nil)
	c1 := &http.Client{Jar: jar1}
	doPost(c1, "/api/setup", map[string]string{"username": "admin", "password": "Admin#1234"})
	_, enroll := doPost(c1, "/api/auth/totp/enroll", nil)
	secret, _ := enroll["secret"].(string)
	code, _ := totp.GenerateCode(secret, time.Now())
	if st, _ := doPost(c1, "/api/auth/totp/activate", map[string]string{"code": code}); st != 200 {
		t.Fatalf("activate failed: %d", st)
	}

	// Attacker with only the password gets a StageTOTP cookie.
	jar2, _ := cookiejar.New(nil)
	c2 := &http.Client{Jar: jar2}
	if st, r := doPost(c2, "/api/auth/login", map[string]string{"username": "admin", "password": "Admin#1234"}); st != 200 || r["next"] != "needs_totp_code" {
		t.Fatalf("login next=%v status=%d", r["next"], st)
	}

	// The bypass attempts must be rejected.
	if st, _ := doPost(c2, "/api/auth/totp/enroll", nil); st != http.StatusUnauthorized {
		t.Fatalf("expected re-enroll blocked (401), got %d", st)
	}
	if st, _ := doPost(c2, "/api/auth/totp/activate", map[string]string{"code": "000000"}); st != http.StatusUnauthorized {
		t.Fatalf("expected activate blocked (401), got %d", st)
	}

	// Legitimate verify with the real code still works.
	good, _ := totp.GenerateCode(secret, time.Now())
	if st, r := doPost(c2, "/api/auth/totp/verify", map[string]string{"code": good}); st != 200 || r["next"] != "done" {
		t.Fatalf("verify failed: status=%d next=%v", st, r["next"])
	}
}

// TestStageTokenRevokedOnEpochBump verifies a half-authenticated (StageEnroll)
// session is invalidated when the user's TokenEpoch changes (password reset).
func TestStageTokenRevokedOnEpochBump(t *testing.T) {
	t.Setenv("PANEL_DATA_DIR", t.TempDir())
	t.Setenv("PANEL_NGINX_DRYRUN", "true")
	t.Setenv("PANEL_COOKIE_SECURE", "false")
	t.Setenv("PANEL_JWT_SECRET", "stage-epoch-test-secret-1234567890ab")

	cfg, _ := config.Load()
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app new: %v", err)
	}
	defer application.Close()
	srv := httptest.NewServer(application.Server.Handler)
	defer srv.Close()

	doPost := func(client *http.Client, path string, body any) (int, map[string]any) {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		resp, err := client.Post(srv.URL+path, "application/json", &buf)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	// admin setup + TOTP -> full session
	adminJar, _ := cookiejar.New(nil)
	admin := &http.Client{Jar: adminJar}
	doPost(admin, "/api/setup", map[string]string{"username": "admin", "password": "Admin#1234"})
	_, en := doPost(admin, "/api/auth/totp/enroll", nil)
	code, _ := totp.GenerateCode(en["secret"].(string), time.Now())
	doPost(admin, "/api/auth/totp/activate", map[string]string{"code": code})

	// create bob and find his id
	doPost(admin, "/api/users", map[string]any{"username": "bob", "password": "Bob#12345", "role": "user"})
	resp, _ := admin.Get(srv.URL + "/api/users")
	var list map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	var bobID int
	for _, it := range list["items"].([]any) {
		if m := it.(map[string]any); m["username"] == "bob" {
			bobID = int(m["id"].(float64))
		}
	}

	// bob logs in (password) -> StageEnroll cookie; enroll works initially
	bobJar, _ := cookiejar.New(nil)
	bob := &http.Client{Jar: bobJar}
	if st, r := doPost(bob, "/api/auth/login", map[string]string{"username": "bob", "password": "Bob#12345"}); st != 200 || r["next"] != "needs_totp_enroll" {
		t.Fatalf("bob login: status=%d next=%v", st, r["next"])
	}
	if st, _ := doPost(bob, "/api/auth/totp/enroll", nil); st != 200 {
		t.Fatalf("baseline enroll should work, got %d", st)
	}

	// admin resets bob's password -> bumps bob's TokenEpoch
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(map[string]string{"newPassword": "New#12345"})
	req, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/users/%d/password", srv.URL, bobID), &buf)
	req.Header.Set("Content-Type", "application/json")
	r2, _ := admin.Do(req)
	r2.Body.Close()

	// bob's stale StageEnroll cookie must now be rejected
	if st, _ := doPost(bob, "/api/auth/totp/enroll", nil); st != http.StatusUnauthorized {
		t.Fatalf("expected stale stage token rejected (401), got %d", st)
	}
}

// TestAdminCannotManageOtherAdmin verifies an admin cannot reset another
// admin's password or TOTP, but can manage normal users.
func TestAdminCannotManageOtherAdmin(t *testing.T) {
	t.Setenv("PANEL_DATA_DIR", t.TempDir())
	t.Setenv("PANEL_NGINX_DRYRUN", "true")
	t.Setenv("PANEL_COOKIE_SECURE", "false")
	t.Setenv("PANEL_JWT_SECRET", "admin-rbac-test-secret-1234567890ab")

	cfg, _ := config.Load()
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app new: %v", err)
	}
	defer application.Close()
	srv := httptest.NewServer(application.Server.Handler)
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	post := func(path string, body any) (int, map[string]any) {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		resp, err := client.Post(srv.URL+path, "application/json", &buf)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	put := func(path string, body any) int {
		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(body)
		req, _ := http.NewRequest(http.MethodPut, srv.URL+path, &buf)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("PUT %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	// admin1 setup + TOTP
	post("/api/setup", map[string]string{"username": "admin1", "password": "Admin#1234"})
	_, enroll := post("/api/auth/totp/enroll", nil)
	code, _ := totp.GenerateCode(enroll["secret"].(string), time.Now())
	post("/api/auth/totp/activate", map[string]string{"code": code})

	// create another admin and a normal user
	if st, _ := post("/api/users", map[string]any{"username": "admin2", "password": "Admin#1234", "role": "admin"}); st != 200 {
		t.Fatalf("create admin2: %d", st)
	}
	if st, _ := post("/api/users", map[string]any{"username": "bob", "password": "Bob#12345", "role": "user"}); st != 200 {
		t.Fatalf("create bob: %d", st)
	}

	// fetch ids
	resp, _ := client.Get(srv.URL + "/api/users")
	var list map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	var admin2ID, bobID int
	for _, it := range list["items"].([]any) {
		m := it.(map[string]any)
		switch m["username"] {
		case "admin2":
			admin2ID = int(m["id"].(float64))
		case "bob":
			bobID = int(m["id"].(float64))
		}
	}

	// cannot reset another admin's password or TOTP
	if st := put(fmt.Sprintf("/api/users/%d/password", admin2ID), map[string]string{"newPassword": "New#12345"}); st != http.StatusForbidden {
		t.Fatalf("expected 403 resetting admin2 password, got %d", st)
	}
	if st, _ := post(fmt.Sprintf("/api/users/%d/totp/reset", admin2ID), nil); st != http.StatusForbidden {
		t.Fatalf("expected 403 resetting admin2 totp, got %d", st)
	}
	// but can reset a normal user's password
	if st := put(fmt.Sprintf("/api/users/%d/password", bobID), map[string]string{"newPassword": "New#12345"}); st != 200 {
		t.Fatalf("expected 200 resetting bob password, got %d", st)
	}
}

// TestRequireAuthBlocksWithoutSession ensures business routes reject anonymous calls.
func TestRequireAuthBlocksWithoutSession(t *testing.T) {
	t.Setenv("PANEL_DATA_DIR", t.TempDir())
	t.Setenv("PANEL_NGINX_DRYRUN", "true")
	t.Setenv("PANEL_COOKIE_SECURE", "false")
	t.Setenv("PANEL_JWT_SECRET", "another-test-secret-1234567890ab")

	cfg, _ := config.Load()
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app new: %v", err)
	}
	defer application.Close()
	srv := httptest.NewServer(application.Server.Handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/sites")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session, got %d", resp.StatusCode)
	}
}
