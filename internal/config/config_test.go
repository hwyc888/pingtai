package config

import (
	"strings"
	"testing"
)

func TestDefaultAuthModeAndSQLitePath(t *testing.T) {
	cfg := Defaults()
	if cfg.AuthMode != "local" {
		t.Fatalf("default auth mode = %q, want local", cfg.AuthMode)
	}
	if !strings.HasSuffix(cfg.DatabasePath, "data/pingtai.db") {
		t.Fatalf("unexpected default SQLite path: %s", cfg.DatabasePath)
	}
	cfg.JWTSecret = strings.Repeat("s", 40)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default local configuration invalid: %v", err)
	}
}

func TestAuthModeRequiresCompleteSSOSettings(t *testing.T) {
	cfg := Defaults()
	cfg.JWTSecret = strings.Repeat("s", 40)
	cfg.AuthMode = "sso"
	if err := cfg.Validate(); err == nil {
		t.Fatal("SSO mode without credentials must be rejected")
	}
	cfg.SSOClientID = "client"
	cfg.SSOClientSecret = "secret"
	cfg.SSORedirectURL = "https://platform.example/auth/sso/callback"
	cfg.SSOAdminAccounts = "admin"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("complete SSO configuration rejected: %v", err)
	}
	if !cfg.SSOConfigured() {
		t.Fatal("explicit SSO mode should be enabled")
	}
	cfg.AuthMode = "local"
	if cfg.SSOConfigured() {
		t.Fatal("SSO credentials alone must not switch the active auth mode")
	}
}
