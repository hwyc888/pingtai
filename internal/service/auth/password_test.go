package auth

import (
	"crypto/md5"
	"encoding/hex"
	"testing"
)

func TestVerifyLegacyMD5Password(t *testing.T) {
	sum := md5.Sum([]byte("legacy-pass"))
	stored := hex.EncodeToString(sum[:])
	valid, upgrade := VerifyPassword(stored, "legacy-pass")
	if !valid || !upgrade {
		t.Fatalf("expected valid legacy password requiring upgrade, got valid=%v upgrade=%v", valid, upgrade)
	}
	valid, _ = VerifyPassword(stored, "wrong")
	if valid {
		t.Fatal("wrong password must not validate")
	}
}

func TestBcryptPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	valid, upgrade := VerifyPassword(hash, "correct-horse-battery")
	if !valid || upgrade {
		t.Fatalf("expected valid bcrypt password without upgrade, got valid=%v upgrade=%v", valid, upgrade)
	}
}
