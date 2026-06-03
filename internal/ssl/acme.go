package ssl

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/http/webroot"
	"github.com/go-acme/lego/v4/registration"
)

// Manager performs ACME issuance/renewal via lego's HTTP-01 webroot provider.
type Manager struct {
	CertsDir     string
	AcmeWebroot  string
	AccountsDir  string
	DefaultEmail string
	Staging      bool // default environment when a site doesn't specify one
}

func NewManager(certsDir, acmeWebroot, accountsDir, defaultEmail string, staging bool) *Manager {
	return &Manager{
		CertsDir:     certsDir,
		AcmeWebroot:  acmeWebroot,
		AccountsDir:  accountsDir,
		DefaultEmail: defaultEmail,
		Staging:      staging,
	}
}

func directoryURL(env string) string {
	if env == "production" {
		return lego.LEDirectoryProduction
	}
	return lego.LEDirectoryStaging
}

// ResolveEnv returns the effective ACME environment for a site value.
func (m *Manager) ResolveEnv(requested string) string {
	if requested == "production" || requested == "staging" {
		return requested
	}
	if m.Staging {
		return "staging"
	}
	return "production"
}

// Issue obtains (or renews) a certificate for domains and writes it to the
// shared certs volume. The caller MUST ensure nginx already serves the ACME
// webroot for these domains on port 80 before calling.
//
// ctx is honored at phase boundaries (fail-fast if the caller already cancelled
// or timed out); lego's Obtain itself isn't context-cancellable, but each HTTP
// request it makes is bounded by the lego client's default 30s timeout.
func (m *Manager) Issue(ctx context.Context, domains []string, email, env, certPath, keyPath string) (*CertInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(domains) == 0 {
		return nil, errors.New("未提供域名")
	}
	if email == "" {
		email = m.DefaultEmail
	}
	if email == "" {
		return nil, errors.New("请提供 ACME 申请邮箱")
	}
	env = m.ResolveEnv(env)

	user, err := loadOrCreateAccount(m.AccountsDir, env, email)
	if err != nil {
		return nil, fmt.Errorf("加载 ACME 账户失败: %w", err)
	}

	cfg := lego.NewConfig(user)
	cfg.CADirURL = directoryURL(env)
	cfg.Certificate.KeyType = certcrypto.RSA2048

	client, err := lego.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 ACME 客户端失败: %w", err)
	}

	prov, err := webroot.NewHTTPProvider(m.AcmeWebroot)
	if err != nil {
		return nil, fmt.Errorf("初始化 HTTP-01 验证失败: %w", err)
	}
	if err := client.Challenge.SetHTTP01Provider(prov); err != nil {
		return nil, err
	}

	if user.Registration == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return nil, fmt.Errorf("ACME 账户注册失败: %w", err)
		}
		user.Registration = reg
		if err := saveAccount(m.AccountsDir, env, user); err != nil {
			return nil, err
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err // caller gave up before the (slow) order — don't start it
	}
	res, err := client.Certificate.Obtain(certificate.ObtainRequest{Domains: domains, Bundle: true})
	if err != nil {
		return nil, fmt.Errorf("证书签发失败: %w", err)
	}

	if err := writeCertKeyPair(certPath, keyPath, res.Certificate, res.PrivateKey); err != nil {
		return nil, err
	}

	info, err := ParseCertInfo(res.Certificate)
	if err != nil {
		return nil, err
	}
	info.CertPath, info.KeyPath = certPath, keyPath
	return info, nil
}
