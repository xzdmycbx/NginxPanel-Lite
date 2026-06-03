package ssl

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// NamedCertFiles returns the fullchain/privkey paths for a global named cert.
func NamedCertFiles(certsDir string, certID uint) (cert, key string) {
	d := filepath.Join(certsDir, fmt.Sprintf("cert-%d", certID))
	return filepath.Join(d, "fullchain.pem"), filepath.Join(d, "privkey.pem")
}

// CertDir returns a named cert's directory (for deletion).
func CertDir(certsDir string, certID uint) string {
	return filepath.Join(certsDir, fmt.Sprintf("cert-%d", certID))
}

// writeCertKeyPair replaces a site's cert+key as one unit: both are staged to
// temp files first (so a write failure leaves the existing pair untouched), then
// swapped in; if the key swap fails, the cert swap is rolled back. This ensures
// the on-disk fullchain/privkey are never left mismatched (which would make a
// later `nginx -t` fail and could strand an HTTPS site on plain :80).
func writeCertKeyPair(certPath, keyPath string, certPEM, keyPEM []byte) error {
	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		return err
	}
	certTmp, keyTmp := certPath+".tmp", keyPath+".tmp"
	if err := os.WriteFile(certTmp, certPEM, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(keyTmp, keyPEM, 0o600); err != nil {
		_ = os.Remove(certTmp)
		return err
	}
	// Snapshot the current cert so a failed key swap can be undone.
	certBak := certPath + ".bak"
	_ = os.Remove(certBak)
	hadCert := os.Rename(certPath, certBak) == nil
	if err := os.Rename(certTmp, certPath); err != nil {
		if hadCert {
			_ = os.Rename(certBak, certPath)
		}
		_ = os.Remove(keyTmp)
		return err
	}
	if err := os.Rename(keyTmp, keyPath); err != nil {
		_ = os.Remove(certPath) // undo the cert swap to keep cert+key matched
		if hadCert {
			_ = os.Rename(certBak, certPath)
		}
		return err
	}
	_ = os.Remove(certBak)
	return nil
}

// emailHash returns a short stable directory name for an ACME account email.
func emailHash(email string) string {
	sum := sha256.Sum256([]byte(email))
	return hex.EncodeToString(sum[:])[:16]
}
