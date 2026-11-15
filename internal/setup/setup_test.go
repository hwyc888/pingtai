package setup

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pingtai/internal/config"
)

func setupRequest(handler *Handler, method, body, token, origin string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://localhost/setup/api", strings.NewReader(body))
	request.Header.Set("X-Setup-Request", "1")
	request.Header.Set("X-Setup-Token", token)
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestSetupProtectsCredentialsAndRejectsCrossOriginRequests(t *testing.T) {
	current := config.Defaults()
	current.DatabasePath = filepath.Join(t.TempDir(), "data", "test.db")
	current.JWTSecret = strings.Repeat("jwt-secret", 5)
	current.CTYunAPIKey = "private-model-key"
	current.OpenAIAPIKey = "private-openai-key"
	current.CozePrivateKey = "private-pem-content"
	handler := New(current, nil)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		response := setupRequest(handler, method, "{}", "wrong-token", "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s request without a valid token returned %d", method, response.Code)
		}
		response = setupRequest(handler, method, "{}", handler.BootstrapToken(), "https://attacker.example")
		if response.Code != http.StatusForbidden {
			t.Fatalf("cross-origin %s request returned %d", method, response.Code)
		}
	}
	response := setupRequest(handler, http.MethodGet, "", handler.BootstrapToken(), "http://localhost")
	if response.Code != http.StatusOK {
		t.Fatalf("authorized read returned %d", response.Code)
	}
	for _, secret := range []string{current.JWTSecret, current.CTYunAPIKey, current.OpenAIAPIKey, current.CozePrivateKey} {
		if strings.Contains(response.Body.String(), secret) {
			t.Fatal("a saved credential was returned by the configuration API")
		}
	}
}

func TestSetupSavePreservesSecretsAndRequiresRestart(t *testing.T) {
	t.Setenv("PINGTAI_CONFIG_FILE", filepath.Join(t.TempDir(), "pingtai-config.json"))
	current := config.Defaults()
	current.DatabasePath = filepath.Join(t.TempDir(), "data", "test.db")
	current.JWTSecret = strings.Repeat("jwt-secret", 5)
	current.CTYunAPIKey = "preserved-key"
	current.OpenAIAPIKey = "preserved-openai-key"
	current.CozeAccessToken = "removed-token"
	handler := New(current, nil)
	handler.prepare = func(_ context.Context, candidate config.Config, username, password string) error {
		if candidate.DatabasePath != current.DatabasePath || username != "admin" || password != "admin-password" {
			t.Fatal("the database password or administrator inputs did not reach initialization")
		}
		return nil
	}
	body := `{"Settings":{"CTYunAPIKey":"","OpenAIAPIKey":"","JWTSecret":"","CTYunModel":"new-model"},"ClearSecrets":["CozeAccessToken"],"AdminUsername":"admin","AdminPassword":"admin-password"}`
	response := setupRequest(handler, http.MethodPost, body, handler.BootstrapToken(), "http://localhost")
	if response.Code != http.StatusOK {
		t.Fatalf("save failed: %d %s", response.Code, response.Body.String())
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CTYunAPIKey != current.CTYunAPIKey || loaded.OpenAIAPIKey != current.OpenAIAPIKey || loaded.JWTSecret != current.JWTSecret || loaded.CozeAccessToken != "" || loaded.CTYunModel != "new-model" {
		t.Fatal("preserved, cleared or updated settings were not persisted correctly")
	}
	if handler.current.CTYunModel != current.CTYunModel {
		t.Fatal("saving configuration unexpectedly mutated the active runtime configuration")
	}
	response = setupRequest(handler, http.MethodPost, body, handler.BootstrapToken(), "")
	if response.Code != http.StatusConflict {
		t.Fatal("setup accepted another write before the required restart")
	}
}

func TestFailedDatabaseValidationDoesNotOverwriteSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pingtai-config.json")
	t.Setenv("PINGTAI_CONFIG_FILE", path)
	current := config.Defaults()
	current.DatabasePath = filepath.Join(t.TempDir(), "data", "test.db")
	current.JWTSecret = strings.Repeat("secret", 8)
	if err := config.Save(current); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(current, nil)
	handler.prepare = func(context.Context, config.Config, string, string) error {
		return errors.New("database validation failed")
	}
	request := saveRequest{}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response := setupRequest(handler, http.MethodPost, string(body), handler.BootstrapToken(), "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("failed database validation returned %d", response.Code)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) || handler.pending {
		t.Fatal("failed validation changed the saved file or locked out a retry")
	}
}
