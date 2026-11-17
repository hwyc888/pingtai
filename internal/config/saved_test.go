package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestSavedConfigurationSurvivesReloadAndOverridesEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pingtai-config.json")
	t.Setenv("PINGTAI_CONFIG_FILE", path)
	t.Setenv("PINGTAI_DB_PATH", filepath.Join(t.TempDir(), "stale.db"))
	t.Setenv("JWT_SECRET_KEY", "stale-environment-secret")
	saved := Defaults()
	saved.DatabasePath = filepath.Join(t.TempDir(), "saved.db")
	if err := os.WriteFile(saved.DatabasePath, []byte("existing sqlite database"), 0600); err != nil {
		t.Fatal(err)
	}
	saved.JWTSecret = strings.Repeat("secret", 8)
	saved.CTYunAPIKey = "saved-model-key"
	saved.CTYunSystemPrompt = "first line\nsecond line"
	saved.OpenAIModels = []string{"model-one", "model-two"}
	if err := Save(saved); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, saved) {
		t.Fatal("web settings were not loaded intact or were overridden by the environment")
	}
	saved.CTYunModel = "updated-model"
	if err := Save(saved); err != nil {
		t.Fatalf("cannot replace existing settings: %v", err)
	}
	loaded, err = Load()
	if err != nil || loaded.CTYunModel != saved.CTYunModel {
		t.Fatal("replacement configuration was not persisted")
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	saved.JWTSecret = "short"
	if err := Save(saved); err == nil {
		t.Fatal("invalid configuration was accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatal("a failed save changed the previous configuration")
	}
}

func TestMalformedSavedConfigurationDoesNotFallBackSilently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pingtai-config.json")
	t.Setenv("PINGTAI_CONFIG_FILE", path)
	if err := os.WriteFile(path, []byte("{broken json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("malformed saved configuration must trigger recovery setup")
	}
}

func TestLegacyMySQLConfigurationRequiresExplicitSQLiteMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pingtai-config.json")
	t.Setenv("PINGTAI_CONFIG_FILE", path)
	if err := os.WriteFile(path, []byte(`{"DBDSN":"legacy-mysql-dsn"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "legacy MySQL configuration detected") {
		t.Fatalf("legacy MySQL configuration should require explicit migration, got %v", err)
	}
}

func TestDefaultDatabasePathUsesStableXDGDataHome(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG_DATA_HOME is used by the Linux default path")
	}
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	want := filepath.Join(dataHome, "pingtai", "pingtai.db")
	if got := persistentDatabasePath(); got != want {
		t.Fatalf("persistent database path = %q, want %q", got, want)
	}
}

func TestSavedDatabasePathFallsBackToStableDataAfterReleaseFolderMove(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pingtai-config.json")
	dataHome := t.TempDir()
	t.Setenv("PINGTAI_CONFIG_FILE", configPath)
	t.Setenv("PINGTAI_DB_PATH", "")
	t.Setenv("XDG_DATA_HOME", dataHome)

	stablePath := filepath.Join(dataHome, "pingtai", "pingtai.db")
	if err := os.MkdirAll(filepath.Dir(stablePath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stablePath, []byte("existing stable database"), 0600); err != nil {
		t.Fatal(err)
	}

	saved := Defaults()
	saved.DatabasePath = filepath.Join(t.TempDir(), "old-release", "data", "pingtai.db")
	saved.JWTSecret = strings.Repeat("secret", 8)
	if err := Save(saved); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DatabasePath != stablePath {
		t.Fatalf("database path = %q, want stable path %q", loaded.DatabasePath, stablePath)
	}
}

func TestExistingExplicitDatabasePathWinsWhenPresent(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "pingtai-config.json")
	t.Setenv("PINGTAI_CONFIG_FILE", configPath)
	t.Setenv("PINGTAI_DB_PATH", "")
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	savedPath := filepath.Join(t.TempDir(), "saved.db")
	explicitPath := filepath.Join(t.TempDir(), "explicit.db")
	for _, path := range []string{savedPath, explicitPath} {
		if err := os.WriteFile(path, []byte("existing database"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	saved := Defaults()
	saved.DatabasePath = savedPath
	saved.JWTSecret = strings.Repeat("secret", 8)
	if err := Save(saved); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINGTAI_DB_PATH", explicitPath)
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DatabasePath != explicitPath {
		t.Fatalf("database path = %q, want explicit path %q", loaded.DatabasePath, explicitPath)
	}
}
