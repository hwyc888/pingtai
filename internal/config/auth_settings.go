package config

import (
	"context"
	"database/sql"
	"strconv"
)

type authSetting struct {
	key   string
	value string
}

func LoadAuthSettings(ctx context.Context, db *sql.DB, cfg *Config) error {
	rows, err := db.QueryContext(ctx, "SELECT setting_key, setting_value FROM ai_platform_setting WHERE setting_key IN ('auth_mode','local_login_enabled','sso_login_enabled','sso_base_url','sso_service_url','sso_client_id','sso_client_secret','sso_redirect_url','sso_admin_accounts')")
	if err != nil {
		return err
	}
	defer rows.Close()

	localSeen := false
	ssoSeen := false
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return err
		}
		switch key {
		case "auth_mode":
			cfg.AuthMode = value
		case "local_login_enabled":
			enabled, parseErr := strconv.ParseBool(value)
			if parseErr != nil {
				return parseErr
			}
			cfg.LocalLoginEnabled = enabled
			localSeen = true
		case "sso_login_enabled":
			enabled, parseErr := strconv.ParseBool(value)
			if parseErr != nil {
				return parseErr
			}
			cfg.SSOLoginEnabled = enabled
			ssoSeen = true
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
	if err := rows.Err(); err != nil {
		return err
	}
	// Old installations stored only auth_mode. Keep local accounts available
	// while migrating an old SSO-only setting to the new selectable login page.
	if !localSeen {
		cfg.LocalLoginEnabled = true
	}
	if !ssoSeen {
		cfg.SSOLoginEnabled = cfg.AuthMode == "sso" || cfg.AuthMode == "multi"
	}
	cfg.AuthMode = cfg.LoginMode()
	return nil
}

func SaveAuthSettings(ctx context.Context, db *sql.DB, cfg Config) error {
	settings := []authSetting{
		{key: "auth_mode", value: cfg.LoginMode()},
		{key: "local_login_enabled", value: strconv.FormatBool(cfg.LocalLoginEnabled)},
		{key: "sso_login_enabled", value: strconv.FormatBool(cfg.SSOConfigured())},
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
