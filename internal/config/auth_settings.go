package config

import (
	"context"
	"database/sql"
)

type authSetting struct {
	key   string
	value string
}

func LoadAuthSettings(ctx context.Context, db *sql.DB, cfg *Config) error {
	rows, err := db.QueryContext(ctx, "SELECT setting_key, setting_value FROM ai_platform_setting WHERE setting_key IN ('auth_mode','sso_base_url','sso_service_url','sso_client_id','sso_client_secret','sso_redirect_url','sso_admin_accounts')")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		switch key {
		case "auth_mode":
			cfg.AuthMode = value
		case "sso_base_url":
			cfg.SSOBaseURL = value
		case "sso_service_url":
			cfg.SSOServiceURL = value
		case "sso_client_id":
			cfg.SSOClientID = value
		case "sso_client_secret":
			cfg.SSOClientSecret = value
		case "sso_redirect_url":
			cfg.SSORedirectURL = value
		case "sso_admin_accounts":
			cfg.SSOAdminAccounts = value
		}
	}
	return rows.Err()
}

func SaveAuthSettings(ctx context.Context, db *sql.DB, cfg Config) error {
	settings := []authSetting{
		{key: "auth_mode", value: cfg.AuthMode},
		{key: "sso_base_url", value: cfg.SSOBaseURL},
		{key: "sso_service_url", value: cfg.SSOServiceURL},
		{key: "sso_client_id", value: cfg.SSOClientID},
		{key: "sso_client_secret", value: cfg.SSOClientSecret},
		{key: "sso_redirect_url", value: cfg.SSORedirectURL},
		{key: "sso_admin_accounts", value: cfg.SSOAdminAccounts},
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, setting := range settings {
		if _, err := tx.ExecContext(ctx, "INSERT INTO ai_platform_setting (setting_key, setting_value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP) ON CONFLICT(setting_key) DO UPDATE SET setting_value=excluded.setting_value, updated_at=CURRENT_TIMESTAMP", setting.key, setting.value); err != nil {
			return err
		}
	}
	return tx.Commit()
}
