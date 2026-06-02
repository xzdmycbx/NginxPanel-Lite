// Package ssl handles manual PEM certificates and Let's Encrypt (ACME) issuance
// and renewal. Certificates are written to the shared certs volume; the nginx
// subsystem references them by path.
package ssl

import "time"

// CertInfo summarizes a certificate for the UI and persistence.
type CertInfo struct {
	Domains   []string  `json:"domains"`
	NotBefore time.Time `json:"notBefore"`
	NotAfter  time.Time `json:"notAfter"`
	Issuer    string    `json:"issuer"`
	Subject   string    `json:"subject"`
	CertPath  string    `json:"certPath"`
	KeyPath   string    `json:"keyPath"`
}

// DaysLeft returns whole days until expiry.
func (c CertInfo) DaysLeft() int {
	return int(time.Until(c.NotAfter).Hours() / 24)
}
