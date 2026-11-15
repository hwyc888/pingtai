package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
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

func Defaults() Config {
	return Config{
		Addr: ":5050", ChatProvider: "openai", AccessTokenTTL: 30 * time.Minute,
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
	saved := Defaults()
	if err := json.Unmarshal(contents, &saved); err != nil {
		return Config{}, true, errors.New("web configuration is invalid JSON; reopen /setup to repair it")
	}
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
