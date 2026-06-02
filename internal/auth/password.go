package auth

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword bcrypt-hashes pw. It rejects passwords over 72 bytes because
// bcrypt silently truncates them, which would make distinct long passwords
// hash identically.
func HashPassword(pw string) (string, error) {
	if len([]byte(pw)) > 72 {
		return "", bcrypt.ErrPasswordTooLong
	}
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether pw matches the stored hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// ValidatePassword enforces the password policy and returns a Chinese error on failure.
func ValidatePassword(pw, username string) error {
	n := utf8.RuneCountInString(pw)
	if n < 8 || n > 64 {
		return errors.New("密码长度需为 8-64 个字符")
	}
	if len([]byte(pw)) > 72 {
		return errors.New("密码过长")
	}
	if strings.EqualFold(pw, username) {
		return errors.New("密码不能与用户名相同")
	}
	var lo, up, di, sy int
	for _, r := range pw {
		switch {
		case unicode.IsLower(r):
			lo = 1
		case unicode.IsUpper(r):
			up = 1
		case unicode.IsDigit(r):
			di = 1
		default:
			sy = 1
		}
	}
	if lo+up+di+sy < 3 {
		return errors.New("密码需包含大小写字母、数字、符号中的至少三类")
	}
	return nil
}
