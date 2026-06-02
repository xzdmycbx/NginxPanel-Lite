// Package config loads all runtime configuration from environment variables.
// Every path the nginx / ssl subsystems use is sourced here so nothing is
// hardcoded; see the "path contract" in the project plan.
package config

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// derive32 deterministically derives a 32-byte key from any input string.
func derive32(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

type Config struct {
	ListenAddr string // PANEL_LISTEN_ADDR (default :8080)
	DataDir    string // PANEL_DATA_DIR    (panel-only volume: db, backups, acme accounts)
	DBPath     string // derived: DataDir/panel.db

	JWTSecret []byte        // PANEL_JWT_SECRET (auto-generated+persisted if empty)
	SecretKey []byte        // PANEL_SECRET_KEY for AES-GCM at-rest encryption (derived from JWT secret if empty)
	JWTTTL    time.Duration // PANEL_JWT_TTL (default 2h) — full session lifetime

	CookieSecure bool // PANEL_COOKIE_SECURE (default true; set false for plain-HTTP local dev)

	NginxContainerName string // NGINX_CONTAINER_NAME (default nginx-panel-nginx)
	NginxConfDir       string // PANEL_NGINX_CONF_DIR — root of shared nginx config volume (contains conf.d/, snippets/)
	CertsDir           string // PANEL_CERTS_DIR
	AcmeWebroot        string // PANEL_ACME_WEBROOT
	DockerHost         string // PANEL_DOCKER_HOST (default unix:///var/run/docker.sock)
	NginxDryRun        bool   // PANEL_NGINX_DRYRUN — skip real docker exec (local dev without docker)

	ACMEEmail   string // ACME_EMAIL — default Let's Encrypt contact
	ACMEStaging bool   // ACME_STAGING (default true)
}

// Load reads configuration from the environment, applying defaults and
// ensuring the data directory and signing secrets exist.
func Load() (*Config, error) {
	c := &Config{
		ListenAddr:         env("PANEL_LISTEN_ADDR", ":8080"),
		DataDir:            env("PANEL_DATA_DIR", "./data"),
		JWTTTL:             envDuration("PANEL_JWT_TTL", 2*time.Hour),
		CookieSecure:       envBool("PANEL_COOKIE_SECURE", true),
		NginxContainerName: env("NGINX_CONTAINER_NAME", "nginx-panel-nginx"),
		DockerHost:         env("PANEL_DOCKER_HOST", "unix:///var/run/docker.sock"),
		NginxDryRun:        envBool("PANEL_NGINX_DRYRUN", false),
		ACMEEmail:          env("ACME_EMAIL", ""),
		ACMEStaging:        envBool("ACME_STAGING", true),
	}

	c.DBPath = filepath.Join(c.DataDir, "panel.db")
	c.NginxConfDir = env("PANEL_NGINX_CONF_DIR", filepath.Join(c.DataDir, "nginx-panel"))
	c.CertsDir = env("PANEL_CERTS_DIR", filepath.Join(c.NginxConfDir, "certs"))
	c.AcmeWebroot = env("PANEL_ACME_WEBROOT", filepath.Join(c.DataDir, "acme-webroot"))

	// Ensure base directories exist (best-effort; failures surface on first use).
	for _, d := range []string{
		c.DataDir,
		filepath.Join(c.NginxConfDir, "sites"),
		filepath.Join(c.NginxConfDir, "logs"),
		c.CertsDir, c.AcmeWebroot,
		filepath.Join(c.DataDir, "acme", "accounts"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("create dir %s: %w", d, err)
		}
	}

	secret, err := loadOrCreateSecret(os.Getenv("PANEL_JWT_SECRET"), filepath.Join(c.DataDir, ".jwt_secret"))
	if err != nil {
		return nil, err
	}
	c.JWTSecret = secret

	if raw := os.Getenv("PANEL_SECRET_KEY"); raw != "" {
		c.SecretKey = derive32(raw)
	} else {
		// Derive a stable 32-byte key from the JWT secret so TOTP secrets survive restarts.
		c.SecretKey = derive32("totp-aes-key:" + string(c.JWTSecret))
		log.Printf("[config] 警告: 未设置 PANEL_SECRET_KEY，TOTP 加密密钥派生自 JWT 密钥；生产环境请显式设置 PANEL_SECRET_KEY（与 JWT 密钥分开管理/轮换）")
	}

	return c, nil
}

// loadOrCreateSecret returns the explicit secret if provided, otherwise reads a
// persisted secret from path, generating and persisting a new one if absent.
func loadOrCreateSecret(explicit, path string) ([]byte, error) {
	if explicit != "" {
		if len(explicit) < 16 {
			return nil, fmt.Errorf("PANEL_JWT_SECRET too short (need >=16 chars)")
		}
		return []byte(explicit), nil
	}
	if b, err := os.ReadFile(path); err == nil && len(b) >= 16 {
		return b, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("generate jwt secret: %w", err)
	}
	hexed := []byte(hex.EncodeToString(buf))
	if err := os.WriteFile(path, hexed, 0o600); err != nil {
		return nil, fmt.Errorf("persist jwt secret: %w", err)
	}
	return hexed, nil
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
