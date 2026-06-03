// Package models defines the GORM models — the shared data contract used by
// every other subsystem (auth, nginx, ssl, handlers).
package models

import "time"

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

type SSLMode string

const (
	SSLNone   SSLMode = "none"
	SSLManual SSLMode = "manual"
	SSLACME   SSLMode = "acme"
)

// CertSource is how a global certificate was obtained.
type CertSource string

const (
	CertManual CertSource = "manual" // uploaded PEM pair
	CertACME   CertSource = "acme"   // issued/renewed via Let's Encrypt
)

// Certificate is a named TLS certificate managed on the global SSL page and
// referenced by sites. Decoupling certs from sites lets one cert serve many
// sites and be managed (upload / issue / renew) in one place.
type Certificate struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	Name          string     `gorm:"uniqueIndex;not null" json:"name"`
	Source        CertSource `gorm:"not null" json:"source"`
	Domains       []string   `gorm:"serializer:json" json:"domains"`
	CertPath      string     `json:"-"` // on-disk fullchain (panel + nginx shared volume)
	KeyPath       string     `json:"-"`
	NotAfter      *time.Time `json:"notAfter"`
	Issuer        string     `json:"issuer"`
	ACMEEmail     string     `json:"acmeEmail"`     // acme only
	ACMEEnv       string     `json:"acmeEnv"`       // "staging" | "production"
	LastRenewedAt *time.Time `json:"lastRenewedAt"` // acme only
	RenewError    string     `json:"renewError"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

// ProxyLocation is one nginx location block within a site: a custom path that
// reverse-proxies to one or more upstreams, with per-location options.
type ProxyLocation struct {
	Path             string   `json:"path"`             // e.g. "/", "/api", "= /exact"
	UpstreamTargets  []string `json:"upstreamTargets"`  // proxy_pass backend(s)
	WebsocketUpgrade bool     `json:"websocketUpgrade"` // add Upgrade/Connection headers
	CacheEnabled     bool     `json:"cacheEnabled"`     // enable proxy_cache for this location
	ExtraConfig      string   `json:"extraConfig"`      // raw directives inside this location block
}

// User is a panel account. Username is immutable after creation (no route mutates it).
type User struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	Username string `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string `gorm:"not null" json:"-"`
	Role         Role   `gorm:"not null;default:user" json:"role"`
	// SystemAdmin is the single super-admin created at setup: it can manage every
	// other admin/user (reset password/TOTP, disable). Only one exists unless the
	// DB is edited by hand. A regular admin (Role=admin, SystemAdmin=false) may
	// only manage Role=user accounts.
	SystemAdmin  bool      `gorm:"not null;default:false" json:"systemAdmin"`
	Disabled     bool      `gorm:"not null;default:false" json:"disabled"` // a disabled account cannot log in
	TOTPSecret   string    `gorm:"" json:"-"`                              // AES-GCM encrypted base32 secret
	TOTPPending  string    `gorm:"" json:"-"`                              // encrypted pending secret during a self re-bind
	TOTPEnabled  bool      `gorm:"not null;default:false" json:"totpEnabled"`
	TokenEpoch   int       `gorm:"not null;default:0" json:"-"` // bumped on password change/reset/disable to revoke sessions
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Site is one reverse-proxy server block managed by the panel.
type Site struct {
	ID                 uint            `gorm:"primaryKey" json:"id"`
	Name               string          `gorm:"not null" json:"name"`
	ServerNames        []string        `gorm:"serializer:json" json:"serverNames"`     // domains -> server_name
	Locations          []ProxyLocation `gorm:"serializer:json" json:"locations"`       // reverse-proxy location blocks
	UpstreamTargets    []string        `gorm:"serializer:json" json:"upstreamTargets"` // legacy single-location fallback
	WebsocketUpgrade   bool            `gorm:"not null;default:false" json:"websocketUpgrade"`
	ForceHTTPSRedirect bool            `gorm:"not null;default:true" json:"forceHttpsRedirect"`
	// CertID references a global Certificate; nil means no TLS. CertPath/KeyPath
	// are the resolved on-disk paths (denormalized from the cert) that nginx uses.
	CertID            *uint  `json:"certId"`
	CertPath          string `json:"-"`
	KeyPath           string `json:"-"`
	RawConfigOverride string `json:"rawConfigOverride"` // verbatim nginx snippet (admin only)
	RawEdited          bool       `gorm:"not null;default:false" json:"rawEdited"` // a config file was hand-edited
	Enabled            bool       `gorm:"not null;default:true" json:"enabled"`
	UpdatedByUserID    *uint      `json:"updatedByUserId"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

// EffectiveLocations returns the site's location blocks, synthesizing a single
// "/" location from the legacy UpstreamTargets field for sites created before
// multi-location support.
func (s *Site) EffectiveLocations() []ProxyLocation {
	if len(s.Locations) > 0 {
		return s.Locations
	}
	if len(s.UpstreamTargets) > 0 {
		return []ProxyLocation{{
			Path:             "/",
			UpstreamTargets:  s.UpstreamTargets,
			WebsocketUpgrade: s.WebsocketUpgrade,
		}}
	}
	return nil
}

// AuditLog is a global, read-only operation record visible to all users.
type AuditLog struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ActorUserID   *uint     `gorm:"index" json:"actorUserId"`
	ActorUsername string    `gorm:"index" json:"actorUsername"`
	Action        string    `gorm:"index" json:"action"`     // e.g. site.update
	TargetType    string    `gorm:"index" json:"targetType"` // site|user|cert|auth
	TargetID      string    `json:"targetId"`
	Detail        string    `json:"detail"` // human-readable Chinese summary; never contains secrets
	Result        string    `json:"result"` // ok|error
	IP            string    `json:"ip"`
	RequestID     string    `json:"requestId"`
	CreatedAt     time.Time `gorm:"index" json:"createdAt"`
}

// Setting is a key/value store (first_run flag, acme account refs, etc.).
type Setting struct {
	Key       string    `gorm:"primaryKey" json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// All returns every model for AutoMigrate.
func All() []any {
	return []any{&User{}, &Site{}, &Certificate{}, &AuditLog{}, &Setting{}}
}
