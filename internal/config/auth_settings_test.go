package config

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestAuthSettingsRoundTripThroughSQLite(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("CREATE TABLE ai_platform_setting (setting_key TEXT PRIMARY KEY, setting_value TEXT NOT NULL, updated_at TEXT NOT NULL)"); err != nil {
		t.Fatal(err)
	}

	cfg := Defaults()
	cfg.AuthMode = "sso"
	cfg.SSOBaseURL = "https://ids.zcvc.cn/cas"
	cfg.SSOServiceURL = "https://platform.example/"
	cfg.SSOClientID = "client-id"
	cfg.SSOClientSecret = "client-secret"
	cfg.SSORedirectURL = "https://platform.example/auth/sso/callback"
	cfg.SSOAdminAccounts = "school-admin"
	if err := SaveAuthSettings(context.Background(), db, cfg); err != nil {
		t.Fatal(err)
	}

	loaded := Defaults()
	if err := LoadAuthSettings(context.Background(), db, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.AuthMode != "sso" || loaded.SSOBaseURL != cfg.SSOBaseURL ||
		loaded.SSOServiceURL != cfg.SSOServiceURL || loaded.SSOClientID != cfg.SSOClientID ||
		loaded.SSOClientSecret != cfg.SSOClientSecret || loaded.SSORedirectURL != cfg.SSORedirectURL ||
		loaded.SSOAdminAccounts != cfg.SSOAdminAccounts {
		t.Fatalf("authentication settings did not round-trip: %#v", loaded)
	}
	if len(loaded.JWTSecret) != 0 {
		t.Fatal("auth settings persistence must not overwrite the JWT secret")
	}
	if !strings.Contains(loaded.SSOClientSecret, "client-secret") {
		t.Fatal("SSO client secret was not persisted")
	}
}
