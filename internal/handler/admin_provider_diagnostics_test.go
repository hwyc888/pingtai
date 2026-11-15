package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/model"
	"pingtai/internal/repository"
)

func newProviderActionTestHandler(t *testing.T, upstream string) (*Handler, int64) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	providers := repository.NewProviderRepository(db)
	id, err := providers.Save(ctx, model.Provider{
		Name: "test-upstream", BaseURL: upstream, APIKey: "test-secret",
		DefaultModel: "model-a", TimeoutSeconds: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := providers.ReplaceModels(ctx, id, []model.ProviderModel{{
		ProviderID: id, ModelID: "model-a", Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.OpenAITimeout = 10 * time.Second
	return &Handler{Config: &cfg, DB: db}, id
}

func TestProviderConnectionCheckUsesConfiguredCredentials(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("upstream API key was not forwarded")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"data\":[{\"id\":\"model-a\"},{\"id\":\"model-b\"}]}"))
	}))
	defer upstream.Close()
	app, id := newProviderActionTestHandler(t, upstream.URL+"/v1")
	request := httptest.NewRequest(http.MethodPost, "/admin/providers/test", strings.NewReader(`{"id":`+providerTestID(id)+`}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.TestProviderConnection(response, request)
	var result struct {
		Code int `json:"code"`
		Data struct {
			Reachable bool `json:"reachable"`
			ModelCount int `json:"model_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result.Code != 0 || !result.Data.Reachable || result.Data.ModelCount != 2 {
		t.Fatalf("unexpected connection result: %d %s", response.Code, response.Body.String())
	}
}

func TestProviderModelCheckSendsSmallChatCompletionRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("upstream API key was not forwarded")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode chat request: %v", err)
		}
		if payload["model"] != "model-a" || payload["max_tokens"] != float64(8) {
			t.Errorf("unexpected test request: %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"OK"}}]}`))
	}))
	defer upstream.Close()
	app, id := newProviderActionTestHandler(t, upstream.URL+"/v1")
	request := httptest.NewRequest(http.MethodPost, "/admin/providers/test-model",
		strings.NewReader(`{"id":`+providerTestID(id)+`,"model_id":"model-a"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.TestProviderModel(response, request)
	var result struct {
		Code int `json:"code"`
		Data struct {
			ModelID string `json:"model_id"`
			Preview string `json:"response_preview"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result.Code != 0 || result.Data.ModelID != "model-a" || result.Data.Preview != "OK" {
		t.Fatalf("unexpected model test result: %d %s", response.Code, response.Body.String())
	}
}

func providerTestID(value int64) string {
	return strconv.FormatInt(value, 10)
}
