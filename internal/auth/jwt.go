package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/xzdmycbx/nginxpanel-lite/internal/models"
)

// Stage scopes a token so mandatory TOTP can be enforced at the token layer.
type Stage string

const (
	StageEnroll Stage = "enroll" // password ok, must set up TOTP
	StageTOTP   Stage = "totp"   // password ok, must enter TOTP code
	StageFull   Stage = "full"   // fully authenticated, TOTP passed
)

// Claims is the JWT payload carried in the session cookie.
type Claims struct {
	UserID     uint        `json:"uid"`
	Username   string      `json:"usr"`
	Role       models.Role `json:"role"`
	Stage      Stage       `json:"stage"`
	TOTPPassed bool        `json:"totp_passed"`
	Epoch      int         `json:"epoch"`
	jwt.RegisteredClaims
}

// Issue mints a signed token for the given user, stage and lifetime.
func Issue(u models.User, stage Stage, totpPassed bool, ttl time.Duration, secret []byte) (string, error) {
	now := time.Now()
	c := Claims{
		UserID:     u.ID,
		Username:   u.Username,
		Role:       u.Role,
		Stage:      stage,
		TOTPPassed: totpPassed,
		Epoch:      u.TokenEpoch,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.Username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(secret)
}

// Parse validates a token and returns its claims.
func Parse(token string, secret []byte) (*Claims, error) {
	c := &Claims{}
	_, err := jwt.ParseWithClaims(token, c, func(*jwt.Token) (any, error) {
		return secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, err
	}
	return c, nil
}
