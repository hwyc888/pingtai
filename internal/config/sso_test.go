package config

import (
	"strings"
	"testing"
)

func TestSSOConfigurationAndAdministratorAllowlist(t *testing.T) {
	cfg := Defaults()
	cfg.DBDSN = "user:password@tcp(127.0.0.1:3306)/pingtai?parseTime=true"
	cfg.JWTSecret = strings.Repeat("s", 40)
	cfg.SSOClientID = "client-id"
	cfg.SSOClientSecret = "client-secret"
	cfg.SSORedirectURL = "https://platform.example/auth/sso/callback"
	cfg.SSOAdminAccounts = "admin-01; ADMIN-02\noperator-03"

	if !cfg.SSOConfigured() {
		t.Fatal("complete SSO configuration should be enabled")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid SSO configuration rejected: %v", err)
	}
	if !cfg.IsSSOAdmin("admin-01") || !cfg.IsSSOAdmin("admin-02") || !cfg.IsSSOAdmin("operator-03") {
		t.Fatal("configured SSO administrator account was not recognized")
	}
	if cfg.IsSSOAdmin("ordinary-user") || cfg.IsSSOAdmin("") {
		t.Fatal("ordinary or empty account must not be an administrator")
	}
}

func TestPartialSSOConfigurationIsRejected(t *testing.T) {
	cfg := Defaults()
	cfg.DBDSN = "user:password@tcp(127.0.0.1:3306)/pingtai?parseTime=true"
	cfg.JWTSecret = strings.Repeat("s", 40)
	cfg.SSOClientID = "client-id"
	if err := cfg.Validate(); err == nil {
		t.Fatal("partial SSO configuration must be rejected")
	}
}
