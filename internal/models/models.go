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
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`
	Role         Role      `gorm:"not null;default:user" json:"role"`
	TOTPSecret   string    `gorm:"" json:"-"` // AES-GCM encrypted base32 secret
	TOTPEnabled  bool      `gorm:"not null;default:false" json:"totpEnabled"`
	TokenEpoch   int       `gorm:"not null;default:0" json:"-"` // bumped on password change/reset to revoke sessions
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
	SSLMode            SSLMode    `gorm:"not null;default:none" json:"sslMode"`
	CertNotAfter       *time.Time `json:"certNotAfter"`
	LastRenewedAt      *time.Time `json:"lastRenewedAt"`
	ACMEEmail          string     `json:"acmeEmail"`
	ACMEEnv            string     `json:"acmeEnv"` // "staging" | "production"
	RenewError         string     `json:"renewError"`
	CertPath           string     `json:"certPath"` // resolved fullchain path for nginx
	KeyPath            string     `json:"keyPath"`
	RawConfigOverride  string     `json:"rawConfigOverride"` // verbatim nginx snippet (admin only)
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
	return []any{&User{}, &Site{}, &AuditLog{}, &Setting{}}
}
