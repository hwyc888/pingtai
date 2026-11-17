package config

import (
	"path/filepath"
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
	cfg.SSOServiceURL = "https://platform.example/"
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

func TestLoadRejectsLegacyMySQLEnvironmentUntilMigrated(t *testing.T) {
	t.Setenv("PINGTAI_CONFIG_FILE", filepath.Join(t.TempDir(), "missing-config.json"))
	t.Setenv("MYSQL_DSN", "legacy-user:legacy-pass@tcp(localhost:3306)/legacy")
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "legacy MySQL connection settings detected") {
		t.Fatalf("expected explicit legacy database migration warning, got %v", err)
	}
}

func TestModelContextTokensCannotBeNegative(t *testing.T) {
	cfg := Defaults()
	cfg.JWTSecret = strings.Repeat("s", 40)
	cfg.ModelContextTokens = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("negative model context metadata must be rejected")
	}
}


func TestMultipleLoginMethodsCanBeEnabledTogether(t *testing.T) {
	cfg := Defaults()
	cfg.JWTSecret = strings.Repeat("s", 40)
	cfg.AuthMode = "multi"
	cfg.LocalLoginEnabled = true
	cfg.SSOLoginEnabled = true
	cfg.SSOClientID = "client"
	cfg.SSOClientSecret = "secret"
	cfg.SSORedirectURL = "https://platform.example/auth/sso/callback"
	cfg.SSOServiceURL = "https://platform.example/"
	cfg.SSOAdminAccounts = "school-admin"

	if err := cfg.Validate(); err != nil {
		t.Fatalf("both login methods should be valid: %v", err)
	}
	if cfg.LoginMode() != "multi" || !cfg.SSOConfigured() {
		t.Fatalf("both methods should be reported as enabled: %#v", cfg)
	}

	cfg.LocalLoginEnabled = false
	if err := cfg.Validate(); err != nil {
		t.Fatalf("SSO-only mode should remain valid: %v", err)
	}
	if cfg.LoginMode() != "sso" {
		t.Fatalf("expected SSO-only mode, got %q", cfg.LoginMode())
	}

	cfg.SSOLoginEnabled = false
	cfg.AuthMode = "local"
	if err := cfg.Validate(); err == nil {
		t.Fatal("disabling both login methods must be rejected")
	}
}
