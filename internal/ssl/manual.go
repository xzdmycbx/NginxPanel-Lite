package ssl

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

// ValidatePEMPair verifies the cert/key pair matches and parses cert metadata.
func ValidatePEMPair(certPEM, keyPEM []byte) (*CertInfo, error) {
	// X509KeyPair cross-checks that the private key matches the leaf cert.
	if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
		return nil, fmt.Errorf("证书与私钥不匹配: %w", err)
	}
	info, err := ParseCertInfo(certPEM)
	if err != nil {
		return nil, err
	}
	if time.Now().After(info.NotAfter) {
		return nil, errors.New("证书已过期")
	}
	return info, nil
}

// ParseCertInfo extracts SANs/validity/issuer from a (possibly chained) PEM.
func ParseCertInfo(certPEM []byte) (*CertInfo, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("无效的证书 PEM")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("解析证书失败: %w", err)
	}
	return &CertInfo{
		Domains:   leaf.DNSNames,
		NotBefore: leaf.NotBefore,
		NotAfter:  leaf.NotAfter,
		Issuer:    leaf.Issuer.CommonName,
		Subject:   leaf.Subject.CommonName,
	}, nil
}

// StoreManualCert validates an uploaded cert/key pair and writes it atomically
// to the given paths, returning the parsed info with the paths populated.
func StoreManualCert(certPath, keyPath string, certPEM, keyPEM []byte) (*CertInfo, error) {
	info, err := ValidatePEMPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	if err := writeCertKeyPair(certPath, keyPath, certPEM, keyPEM); err != nil {
		return nil, err
	}
	info.CertPath, info.KeyPath = certPath, keyPath
	return info, nil
}
