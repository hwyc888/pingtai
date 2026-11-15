package auth

import (
	"testing"
	"time"
)

func TestTokenPairAndRevocation(t *testing.T) {
	manager := NewTokenManager("test-secret-with-at-least-32-characters-long", time.Hour, 24*time.Hour)
	pair, err := manager.IssuePair("admin")
	if err != nil {
		t.Fatal(err)
	}
	access, err := manager.Parse(pair.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if access.Subject != "admin" || access.Type != "access" {
		t.Fatalf("unexpected access claims: %+v", access)
	}
	refresh, err := manager.Parse(pair.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if refresh.Type != "refresh" {
		t.Fatalf("expected refresh token, got %q", refresh.Type)
	}
	manager.Revoke(access.JTI, access.Expires)
	if _, err := manager.Parse(pair.AccessToken); err == nil {
		t.Fatal("revoked access token must be rejected")
	}
}
