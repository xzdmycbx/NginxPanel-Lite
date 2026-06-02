package nginx

import (
	"bytes"
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
)

//go:embed templates/site.conf.tmpl
var siteTmplText string

//go:embed templates/location.conf.tmpl
var locationTmplText string

var (
	siteTmpl     = template.Must(template.New("site").Parse(siteTmplText))
	locationTmpl = template.Must(template.New("location").Parse(locationTmplText))
)

// NamedFile is one location file (slug -> content).
type NamedFile struct {
	Slug    string
	Content []byte
}

// SiteFiles is the full rendered fileset for a site.
type SiteFiles struct {
	SiteConf  []byte
	Locations []NamedFile
}

type siteData struct {
	Slug               string
	ServerNames        string
	SSLEnabled         bool
	ForceHTTPSRedirect bool
	CertPath           string
	KeyPath            string
	AcmeWebroot        string
	AccessLog          string
	ErrorLog           string
	LocationsGlob      string
	UpstreamBlocks     []string
	RawConfigOverride  string
}

type locationData struct {
	Slug             string
	Path             string
	ProxyPass        string
	WebsocketUpgrade bool
	CacheEnabled     bool
	ExtraConfig      string
}

// Render produces the full fileset (domain main file + one file per location).
// When enableTLS is false, only the plain :80 server is emitted.
func Render(site *models.Site, p Paths, enableTLS bool) (SiteFiles, error) {
	if len(site.ServerNames) == 0 {
		return SiteFiles{}, fmt.Errorf("站点未配置域名")
	}
	locs := site.EffectiveLocations()
	if len(locs) == 0 {
		return SiteFiles{}, fmt.Errorf("站点未配置反向代理")
	}

	var upstreamBlocks []string
	var locFiles []NamedFile
	for i, l := range locs {
		if len(l.UpstreamTargets) == 0 {
			return SiteFiles{}, fmt.Errorf("反向代理路径 %q 未配置目标", l.Path)
		}
		path := strings.TrimSpace(l.Path)
		if path == "" {
			path = "/"
		}
		proxyPass, upstreamBlock := buildLocationUpstream(site.ID, i, l.UpstreamTargets)
		if upstreamBlock != "" {
			upstreamBlocks = append(upstreamBlocks, upstreamBlock)
		}
		slug := locationSlug(path, i)
		content, err := renderLocation(locationData{
			Slug:             Slug(site.ID),
			Path:             path,
			ProxyPass:        proxyPass,
			WebsocketUpgrade: l.WebsocketUpgrade,
			CacheEnabled:     l.CacheEnabled,
			ExtraConfig:      strings.TrimSpace(l.ExtraConfig),
		})
		if err != nil {
			return SiteFiles{}, err
		}
		locFiles = append(locFiles, NamedFile{Slug: slug, Content: content})
	}

	certPath, keyPath := site.CertPath, site.KeyPath
	if certPath == "" || keyPath == "" {
		certPath, keyPath = p.CertFiles(site.ID)
	}

	data := siteData{
		Slug:               Slug(site.ID),
		ServerNames:        strings.Join(site.ServerNames, " "),
		SSLEnabled:         enableTLS,
		ForceHTTPSRedirect: site.ForceHTTPSRedirect,
		CertPath:           filepath.ToSlash(certPath),
		KeyPath:            filepath.ToSlash(keyPath),
		AcmeWebroot:        filepath.ToSlash(p.AcmeWebroot),
		AccessLog:          filepath.ToSlash(p.AccessLog(site.ID)),
		ErrorLog:           filepath.ToSlash(p.ErrorLog(site.ID)),
		LocationsGlob:      p.LocationsGlob(site.ID),
		UpstreamBlocks:     upstreamBlocks,
		RawConfigOverride:  strings.TrimSpace(site.RawConfigOverride),
	}

	var buf bytes.Buffer
	if err := siteTmpl.Execute(&buf, data); err != nil {
		return SiteFiles{}, err
	}
	return SiteFiles{SiteConf: buf.Bytes(), Locations: locFiles}, nil
}

func renderLocation(d locationData) ([]byte, error) {
	var buf bytes.Buffer
	if err := locationTmpl.Execute(&buf, d); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// locationSlug derives a filename-safe, stable, unique slug from a path + index.
func locationSlug(path string, idx int) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(path) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	base := strings.Trim(b.String(), "-")
	if base == "" {
		base = "root"
	}
	if len(base) > 40 {
		base = base[:40]
	}
	return fmt.Sprintf("%02d-%s", idx, base)
}

// buildLocationUpstream returns the proxy_pass target and an optional upstream{}
// block for one location.
func buildLocationUpstream(siteID uint, idx int, targets []string) (proxyPass, upstreamBlock string) {
	if len(targets) == 1 {
		return normalizeTarget(targets[0]), ""
	}
	name := fmt.Sprintf("site_%d_loc%d_backend", siteID, idx)
	var b strings.Builder
	fmt.Fprintf(&b, "upstream %s {\n", name)
	for _, t := range targets {
		fmt.Fprintf(&b, "    server %s;\n", stripScheme(t))
	}
	b.WriteString("}")
	return "http://" + name, b.String()
}

func normalizeTarget(t string) string {
	t = strings.TrimSpace(t)
	if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
		return t
	}
	return "http://" + t
}

func stripScheme(t string) string {
	t = strings.TrimSpace(t)
	t = strings.TrimPrefix(t, "http://")
	t = strings.TrimPrefix(t, "https://")
	return t
}
