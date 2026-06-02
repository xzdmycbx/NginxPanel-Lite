package ssl

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"

	"github.com/go-acme/lego/v4/registration"
)

// acmeUser implements registration.User, persisting its key + registration to disk.
type acmeUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

func (u *acmeUser) GetEmail() string                        { return u.Email }
func (u *acmeUser) GetRegistration() *registration.Resource { return u.Registration }
func (u *acmeUser) GetPrivateKey() crypto.PrivateKey        { return u.key }

func accountDir(accountsDir, env, email string) string {
	return filepath.Join(accountsDir, env, emailHash(email))
}

// loadOrCreateAccount loads a persisted ACME account, or creates a fresh key
// (registration happens later via the lego client).
func loadOrCreateAccount(accountsDir, env, email string) (*acmeUser, error) {
	dir := accountDir(accountsDir, env, email)
	keyPath := filepath.Join(dir, "account.key")
	regPath := filepath.Join(dir, "registration.json")
	u := &acmeUser{Email: email}

	if b, err := os.ReadFile(keyPath); err == nil {
		k, err := parseECKey(b)
		if err != nil {
			return nil, err
		}
		u.key = k
		if rb, err := os.ReadFile(regPath); err == nil {
			var reg registration.Resource
			if json.Unmarshal(rb, &reg) == nil {
				u.Registration = &reg
			}
		}
		return u, nil
	}

	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	u.key = k
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(keyPath, encodeECKey(k), 0o600); err != nil {
		return nil, err
	}
	return u, nil
}

// saveAccount persists the registration resource after a successful Register.
func saveAccount(accountsDir, env string, u *acmeUser) error {
	if u.Registration == nil {
		return nil
	}
	dir := accountDir(accountsDir, env, u.Email)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(u.Registration, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "registration.json"), b, 0o600)
}

func encodeECKey(k *ecdsa.PrivateKey) []byte {
	der, _ := x509.MarshalECPrivateKey(k)
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
}

func parseECKey(b []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("无效的账户私钥")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}
