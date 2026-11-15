package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("PINGTAI_TEST_DOTENV=loaded\nPINGTAI_TEST_PRESERVE=file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PINGTAI_TEST_PRESERVE", "environment")

	loadDotEnv(path)

	if got := os.Getenv("PINGTAI_TEST_DOTENV"); got != "loaded" {
		t.Fatalf("expected dotenv value to load, got %q", got)
	}
	if got := os.Getenv("PINGTAI_TEST_PRESERVE"); got != "environment" {
		t.Fatalf("expected existing environment to win, got %q", got)
	}
}
