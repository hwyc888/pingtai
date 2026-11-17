package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// SettingsPath is independent of the working directory used by a service manager.
func SettingsPath() string {
	if path := strings.TrimSpace(os.Getenv("PINGTAI_CONFIG_FILE")); path != "" {
		return path
	}
	return filepath.Join(executableDir(), "pingtai-config.json")
}

func DefaultDatabasePath() string {
	stable := persistentDatabasePath()
	if databaseFileExists(stable) {
		return stable
	}
	// Keep using the pre-1.0.6 executable-relative database when it still exists.
	// This prevents an in-place upgrade from silently opening a new empty database.
	legacy := legacyDatabasePath()
	if databaseFileExists(legacy) {
		return legacy
	}
	return stable
}

func persistentDatabasePath() string {
	if runtime.GOOS == "windows" {
		if base := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); base != "" {
			return filepath.Join(base, "ChaofanAI", "pingtai", "pingtai.db")
		}
	}
	if runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, "Library", "Application Support", "ChaofanAI", "pingtai", "pingtai.db")
		}
	}
	if base := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); base != "" && filepath.IsAbs(base) {
		return filepath.Join(base, "pingtai", "pingtai.db")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "share", "pingtai", "pingtai.db")
	}
	return filepath.Join(executableDir(), "data", "pingtai.db")
}

func legacyDatabasePath() string {
	return filepath.Join(executableDir(), "data", "pingtai.db")
}

func databaseFileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func resolveSavedDatabasePath(savedPath string) string {
	environmentPath := strings.TrimSpace(os.Getenv("PINGTAI_DB_PATH"))
	if databaseFileExists(environmentPath) {
		return environmentPath
	}
	if databaseFileExists(savedPath) {
		return savedPath
	}
	stable := persistentDatabasePath()
	if databaseFileExists(stable) {
		return stable
	}
	legacy := legacyDatabasePath()
	if databaseFileExists(legacy) {
		return legacy
	}
	if environmentPath != "" {
		return environmentPath
	}
	if strings.TrimSpace(savedPath) != "" {
		return savedPath
	}
	return stable
}

func Defaults() Config {
	return Config{
		Addr: ":5050", AuthMode: "local", LocalLoginEnabled: true, SSOLoginEnabled: false, DatabasePath: DefaultDatabasePath(),
		SSOBaseURL: "https://ids.zcvc.cn/cas", ChatProvider: "openai", AccessTokenTTL: 30 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour, MaxUploadBytes: 100 * 1024 * 1024,
		CozeBaseURL: "https://api.coze.cn", CozeWorkspaceID: "7481824441182617636",
		CTYunAuthMode: "bearer", CTYunRegion: "QG", CTYunSignPrefix: "teleai-cloud-auth-v1",
		CTYunTimeout: 180 * time.Second, CTYunMaxTokens: 1024,
		OpenAIBaseURL: "https://api.openai.com/v1", OpenAITimeout: 180 * time.Second, ContextTurns: 20,
	}
}

func loadSaved() (Config, bool, error) {
	contents, err := os.ReadFile(SettingsPath())
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, true, fmt.Errorf("read web configuration: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(contents, &fields); err != nil {
		return Config{}, true, errors.New("web configuration is invalid JSON; reopen /setup to repair it")
	}
	if _, hasDatabasePath := fields["DatabasePath"]; !hasDatabasePath {
		if _, hasLegacyDSN := fields["DBDSN"]; hasLegacyDSN {
			return Config{}, true, errors.New("legacy MySQL configuration detected; back up and migrate existing data before configuring the SQLite database")
		}
	}
	saved := Defaults()
	if err := json.Unmarshal(contents, &saved); err != nil {
		return Config{}, true, errors.New("web configuration is invalid JSON; reopen /setup to repair it")
	}
	// Upgrade pre-multi-login configuration without disabling local accounts.
	if _, exists := fields["LocalLoginEnabled"]; !exists {
		saved.LocalLoginEnabled = true
	}
	if _, exists := fields["SSOLoginEnabled"]; !exists {
		saved.SSOLoginEnabled = saved.AuthMode == "sso" || saved.AuthMode == "multi"
	}
	// Preserve an existing database first. If an old release directory was removed,
	// fall back to the stable OS data directory or an explicit PINGTAI_DB_PATH.
	saved.DatabasePath = resolveSavedDatabasePath(saved.DatabasePath)
	saved.AuthMode = saved.LoginMode()
	return saved, true, nil
}

func Save(saved Config) error {
	if err := saved.Validate(); err != nil {
		return err
	}
	if _, _, err := net.SplitHostPort(saved.Addr); err != nil {
		return errors.New("HTTP address must use host:port, for example :5050")
	}
	contents, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	target := SettingsPath()
	temporary, err := os.CreateTemp(filepath.Dir(target), ".pingtai-config-*")
	if err != nil {
		return fmt.Errorf("create web configuration: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	defer temporary.Close()
	if _, err := temporary.Write(append(contents, '\n')); err != nil {
		return fmt.Errorf("write web configuration: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, target); err != nil {
		return fmt.Errorf("replace web configuration: %w", err)
	}
	return nil
}
