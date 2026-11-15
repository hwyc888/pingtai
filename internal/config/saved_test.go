package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSavedConfigurationSurvivesReloadAndOverridesEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pingtai-config.json")
	t.Setenv("PINGTAI_CONFIG_FILE", path)
	t.Setenv("MYSQL_DSN", "stale-environment-dsn")
	t.Setenv("JWT_SECRET_KEY", "stale-environment-secret")
	saved := Defaults()
	saved.DBDSN = "webuser:password@tcp(127.0.0.1:3306)/webdb?parseTime=true"
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
