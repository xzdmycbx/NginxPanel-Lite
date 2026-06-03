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

// TestRBACPermissions locks in the system-admin tier: the setup (system) admin
// manages every account; a regular admin may only manage normal users, cannot
// create admins, and cannot disable accounts; a disabled account cannot log in.
func TestRBACPermissions(t *testing.T) {
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

	newClient := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar}
	}
	post := func(c *http.Client, path string, body any) (int, map[string]any) {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		resp, err := c.Post(srv.URL+path, "application/json", &buf)
		if err != nil {
			t.Fatalf("POST %s: %v", path, err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	put := func(c *http.Client, path string, body any) int {
		var buf bytes.Buffer
		_ = json.NewEncoder(&buf).Encode(body)
		req, _ := http.NewRequest(http.MethodPut, srv.URL+path, &buf)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatalf("PUT %s: %v", path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}
	// enrollActivate brings a password-verified session to a full TOTP session.
	enrollActivate := func(c *http.Client) {
		_, en := post(c, "/api/auth/totp/enroll", nil)
		secret, _ := en["secret"].(string)
		code, _ := totp.GenerateCode(secret, time.Now())
		if st, _ := post(c, "/api/auth/totp/activate", map[string]string{"code": code}); st != 200 {
			t.Fatalf("activate failed: %d", st)
		}
	}
	fullLogin := func(username, password string) *http.Client {
		c := newClient()
		st, r := post(c, "/api/auth/login", map[string]string{"username": username, "password": password})
		if st != 200 {
			t.Fatalf("login %s status %d", username, st)
		}
		if r["next"] == "needs_totp_enroll" {
			enrollActivate(c)
		}
		return c
	}
	ids := func(c *http.Client) map[string]int {
		resp, _ := c.Get(srv.URL + "/api/users")
		var list map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&list)
		resp.Body.Close()
		out := map[string]int{}
		for _, it := range list["items"].([]any) {
			m := it.(map[string]any)
			out[m["username"].(string)] = int(m["id"].(float64))
		}
		return out
	}

	// system (super) admin = the setup account
	sys := newClient()
	post(sys, "/api/setup", map[string]string{"username": "admin1", "password": "Admin#1234"})
	enrollActivate(sys)

	for _, u := range []struct{ name, role string }{{"admin2", "admin"}, {"admin3", "admin"}, {"bob", "user"}} {
		if st, _ := post(sys, "/api/users", map[string]any{"username": u.name, "password": "Pass#1234", "role": u.role}); st != 200 {
			t.Fatalf("create %s: %d", u.name, st)
		}
	}
	id := ids(sys)

	// system admin CAN reset another admin's and a user's password
	if st := put(sys, fmt.Sprintf("/api/users/%d/password", id["admin2"]), map[string]string{"newPassword": "Admin2#new1"}); st != 200 {
		t.Fatalf("system admin reset admin2 pwd: want 200 got %d", st)
	}
	if st := put(sys, fmt.Sprintf("/api/users/%d/password", id["bob"]), map[string]string{"newPassword": "Bob#new123"}); st != 200 {
		t.Fatalf("system admin reset bob pwd: want 200 got %d", st)
	}

	// a regular admin (admin2, password just reset) logs in to a full session
	reg := fullLogin("admin2", "Admin2#new1")

	// regular admin CANNOT manage the system admin or another admin
	if st := put(reg, fmt.Sprintf("/api/users/%d/password", id["admin1"]), map[string]string{"newPassword": "x"}); st != http.StatusForbidden {
		t.Fatalf("regular admin reset system-admin pwd: want 403 got %d", st)
	}
	if st := put(reg, fmt.Sprintf("/api/users/%d/password", id["admin3"]), map[string]string{"newPassword": "x"}); st != http.StatusForbidden {
		t.Fatalf("regular admin reset other admin pwd: want 403 got %d", st)
	}
	if st, _ := post(reg, fmt.Sprintf("/api/users/%d/totp/reset", id["admin3"]), nil); st != http.StatusForbidden {
		t.Fatalf("regular admin reset other admin totp: want 403 got %d", st)
	}
	// but CAN manage a normal user
	if st := put(reg, fmt.Sprintf("/api/users/%d/password", id["bob"]), map[string]string{"newPassword": "Bob#new456"}); st != 200 {
		t.Fatalf("regular admin reset user pwd: want 200 got %d", st)
	}
	// regular admin CANNOT create an admin, CAN create a user
	if st, _ := post(reg, "/api/users", map[string]any{"username": "sneaky", "password": "Pass#1234", "role": "admin"}); st != http.StatusForbidden {
		t.Fatalf("regular admin create admin: want 403 got %d", st)
	}
	if st, _ := post(reg, "/api/users", map[string]any{"username": "carol", "password": "Pass#1234", "role": "user"}); st != 200 {
		t.Fatalf("regular admin create user: want 200 got %d", st)
	}
	// disabling is system-admin only
	if st, _ := post(reg, fmt.Sprintf("/api/users/%d/disable", id["bob"]), nil); st != http.StatusForbidden {
		t.Fatalf("regular admin disable: want 403 got %d", st)
	}

	// system admin cannot disable itself, but can disable a regular admin
	if st, _ := post(sys, fmt.Sprintf("/api/users/%d/disable", id["admin1"]), nil); st != http.StatusBadRequest {
		t.Fatalf("system admin disable self: want 400 got %d", st)
	}
	if st, _ := post(sys, fmt.Sprintf("/api/users/%d/disable", id["admin2"]), nil); st != 200 {
		t.Fatalf("system admin disable admin2: want 200 got %d", st)
	}
	// the disabled account can no longer log in
	if st, r := post(newClient(), "/api/auth/login", map[string]string{"username": "admin2", "password": "Admin2#new1"}); st != http.StatusForbidden || r["code"] != "account_disabled" {
		t.Fatalf("disabled login: want 403 account_disabled, got %d %v", st, r["code"])
	}
}

// TestSiteLock verifies a system admin can lock a site with their TOTP, that a
// locked site rejects edits from everyone (incl. the system admin), and that
// unlocking (also TOTP-gated) restores editing. Wrong/missing TOTP is rejected.
func TestSiteLock(t *testing.T) {
	t.Setenv("PANEL_DATA_DIR", t.TempDir())
	t.Setenv("PANEL_NGINX_DRYRUN", "true")
	t.Setenv("PANEL_COOKIE_SECURE", "false")
	t.Setenv("PANEL_JWT_SECRET", "site-lock-test-secret-1234567890abc")

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

	// system admin setup + TOTP (keep the secret to generate lock codes)
	post("/api/setup", map[string]string{"username": "admin", "password": "Admin#1234"})
	_, en := post("/api/auth/totp/enroll", nil)
	secret := en["secret"].(string)
	code, _ := totp.GenerateCode(secret, time.Now())
	post("/api/auth/totp/activate", map[string]string{"code": code})

	body := map[string]any{"name": "demo", "serverNames": []string{"d.example.com"}, "upstreamTargets": []string{"http://app:3000"}}
	st, site := post("/api/sites", body)
	if st != 200 {
		t.Fatalf("create site: %d", st)
	}
	id := int(site["id"].(float64))

	// lock without / with a wrong TOTP code is rejected
	if st, _ := post(fmt.Sprintf("/api/sites/%d/lock", id), map[string]string{"code": "000000"}); st != http.StatusBadRequest {
		t.Fatalf("lock with wrong code: want 400 got %d", st)
	}
	// lock with the real code succeeds
	good, _ := totp.GenerateCode(secret, time.Now())
	if st, _ := post(fmt.Sprintf("/api/sites/%d/lock", id), map[string]string{"code": good}); st != 200 {
		t.Fatalf("lock with code: want 200 got %d", st)
	}
	// a locked site rejects edits even from the system admin
	if st := put(fmt.Sprintf("/api/sites/%d", id), body); st != http.StatusForbidden {
		t.Fatalf("edit locked site: want 403 got %d", st)
	}
	// ...and rejects clearing its logs
	if st, _ := post(fmt.Sprintf("/api/sites/%d/logs/clear?type=access", id), nil); st != http.StatusForbidden {
		t.Fatalf("clear logs on locked site: want 403 got %d", st)
	}
	// unlock (TOTP again) then edit works
	good2, _ := totp.GenerateCode(secret, time.Now())
	if st, _ := post(fmt.Sprintf("/api/sites/%d/unlock", id), map[string]string{"code": good2}); st != 200 {
		t.Fatalf("unlock: want 200 got %d", st)
	}
	if st := put(fmt.Sprintf("/api/sites/%d", id), body); st != 200 {
		t.Fatalf("edit after unlock: want 200 got %d", st)
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
