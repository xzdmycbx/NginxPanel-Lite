package auth

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const totpIssuer = "NginxPanel"

// EnrollResult is returned to the client when setting up TOTP.
type EnrollResult struct {
	OtpauthURL  string `json:"otpauthUrl"`
	SecretB32   string `json:"secret"`      // plaintext base32 — encrypt before persisting
	QRPNGBase64 string `json:"qrDataUri"`   // data:image/png;base64,...
}

// GenerateTOTP creates a new TOTP secret + QR for username.
func GenerateTOTP(username string) (*EnrollResult, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: username,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return nil, err
	}
	img, err := key.Image(256, 256)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return &EnrollResult{
		OtpauthURL:  key.String(),
		SecretB32:   key.Secret(),
		QRPNGBase64: "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()),
	}, nil
}

// ValidateTOTP checks a 6-digit code against the secret with ±1 period skew.
func ValidateTOTP(code, secret string) bool {
	ok, _ := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return ok
}
