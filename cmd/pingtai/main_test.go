package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/model"
	"pingtai/internal/repository"
)

func TestPlatformBootstrapAndProviderConfiguration(t *testing.T) {
	ctx := context.Background()
	sqlitePath := filepath.Join(t.TempDir(), "data", "pingtai.db")

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer verification-upstream-key" {
			t.Errorf("unexpected provider authorization header")
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected upstream URL: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"restart-ok"}}]}`)
	}))
	defer upstream.Close()

	// Seed a provider the same way an administrator configures it in the UI.
	db, err := database.Open(ctx, sqlitePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	providers := repository.NewProviderRepository(db)
	providerID, err := providers.Save(ctx, model.Provider{
		Name: "Integration provider",
		BaseURL: upstream.URL + "/v1",
		APIKey: "verification-upstream-key",
		DefaultModel: "verification-model",
		TimeoutSeconds: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := providers.ReplaceModels(ctx, providerID, []model.ProviderModel{
		{ProviderID: providerID, ModelID: "verification-model", Enabled: true},
		{ProviderID: providerID, ModelID: "second-model", Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := providers.Activate(ctx, providerID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	cfg := config.Defaults()
	cfg.DatabasePath = sqlitePath
	cfg.ModelAPIKey = "verification-platform-key"
	application, cleanup, err := buildApplication(ctx, cfg)
	if err != nil {
		t.Fatalf("application did not start: %v", err)
	}
	defer cleanup()

	health := httptest.NewRecorder()
	application.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health check failed: HTTP %d %s", health.Code, health.Body.String())
	}

	status := httptest.NewRecorder()
	application.ServeHTTP(status, httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil))
	var bootstrapState struct {
		Data struct {
			NeedsAdmin bool `json:"needs_admin"`
		} `json:"data"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &bootstrapState); err != nil || !bootstrapState.Data.NeedsAdmin {
		t.Fatalf("new SQLite database should require first admin: %s err=%v", status.Body.String(), err)
	}

	register := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap/register",
		strings.NewReader(`{"username":"first-admin","password":"verification-admin-password"}`))
	register.Header.Set("Content-Type", "application/json")
	registerResponse := httptest.NewRecorder()
	application.ServeHTTP(registerResponse, register)
	if registerResponse.Code != http.StatusOK {
		t.Fatalf("first admin registration failed: HTTP %d %s", registerResponse.Code, registerResponse.Body.String())
	}
	var token string
	for _, cookie := range registerResponse.Result().Cookies() {
		if cookie.Name == "Authorization" {
			token = cookie.Value
		}
	}
	if token == "" {
		t.Fatal("first admin registration did not establish an access session")
	}

	me := authenticatedGet(t, application, token, "/api/auth/me")
	var profile struct {
		Data struct {
			IsAdmin bool `json:"is_admin"`
		} `json:"data"`
	}
	if err := json.Unmarshal(me.Body.Bytes(), &profile); err != nil || !profile.Data.IsAdmin {
		t.Fatalf("first account did not receive administrator privileges: %s err=%v", me.Body.String(), err)
	}

	modelList := authenticatedGet(t, application, token, "/api/agents/models")
	var models struct {
		Data struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		} `json:"data"`
	}
	if err := json.Unmarshal(modelList.Body.Bytes(), &models); err != nil || len(models.Data.Models) != 2 {
		t.Fatalf("configured models were not exposed to chat: %s err=%v", modelList.Body.String(), err)
	}

	proxyRequest := httptest.NewRequest(http.MethodPost, "/api/v1/chat/completions",
		strings.NewReader(`{"model":"verification-model","messages":[{"role":"user","content":"hello"}]}`))
	proxyRequest.Header.Set("Content-Type", "application/json")
	proxyRequest.Header.Set("X-API-Key", "verification-platform-key")
	proxyResponse := httptest.NewRecorder()
	application.ServeHTTP(proxyResponse, proxyRequest)
	if proxyResponse.Code != http.StatusOK || !strings.Contains(proxyResponse.Body.String(), "restart-ok") {
		t.Fatalf("configured provider could not serve chat completions: HTTP %d %s", proxyResponse.Code, proxyResponse.Body.String())
	}

	secondRegister := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap/register",
		strings.NewReader(`{"username":"second-admin","password":"another-password-123"}`))
	secondRegister.Header.Set("Content-Type", "application/json")
	secondResponse := httptest.NewRecorder()
	application.ServeHTTP(secondResponse, secondRegister)
	if secondResponse.Code != http.StatusConflict {
		t.Fatalf("public first-admin registration should close after first account: HTTP %d %s", secondResponse.Code, secondResponse.Body.String())
	}

	// A fresh process must reuse the JWT signing secret persisted in SQLite.
	cleanup()
	restarted, restartCleanup, err := buildApplication(ctx, cfg)
	if err != nil {
		t.Fatalf("platform could not restart with its existing SQLite database: %v", err)
	}
	defer restartCleanup()
	restartedMe := authenticatedGet(t, restarted, token, "/api/auth/me")
	if restartedMe.Code != http.StatusOK {
		t.Fatalf("access token was invalidated by restart: HTTP %d %s", restartedMe.Code, restartedMe.Body.String())
	}
}

func authenticatedGet(t *testing.T, application http.Handler, token, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	application.ServeHTTP(response, request)
	return response
}
