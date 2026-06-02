package ssl

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// certFiles returns the fullchain/privkey paths for a site under certsDir.
func certFiles(certsDir string, siteID uint) (cert, key string) {
	d := filepath.Join(certsDir, fmt.Sprintf("site-%d", siteID))
	return filepath.Join(d, "fullchain.pem"), filepath.Join(d, "privkey.pem")
}

// atomicWrite writes data to path via a temp file + rename, with the given perm.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// emailHash returns a short stable directory name for an ACME account email.
func emailHash(email string) string {
	sum := sha256.Sum256([]byte(email))
	return hex.EncodeToString(sum[:])[:16]
}
