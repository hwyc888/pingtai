package auth

import (
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func VerifyPassword(stored, password string) (valid bool, needsUpgrade bool) {
	if strings.HasPrefix(stored, "$2a$") || strings.HasPrefix(stored, "$2b$") || strings.HasPrefix(stored, "$2y$") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil, false
	}
	sum := md5.Sum([]byte(password))
	expected := hex.EncodeToString(sum[:])
	if len(stored) != len(expected) {
		return false, false
	}
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(stored)), []byte(expected)) == 1, true
}
