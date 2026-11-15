package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/model"
	"pingtai/internal/repository"
)

func TestBuiltInChatExposesEnabledModelsFromActiveSQLiteProvider(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	providers := repository.NewProviderRepository(db)
	providerID, err := providers.Save(ctx, model.Provider{
		Name: "test-provider",
		BaseURL: "https://api.example.com/v1",
		DefaultModel: "provider/model-b",
		TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := providers.ReplaceModels(ctx, providerID, []model.ProviderModel{
		{ProviderID: providerID, ModelID: "provider/model-a", Enabled: true},
		{ProviderID: providerID, ModelID: "provider/model-b", Enabled: true},
		{ProviderID: providerID, ModelID: "provider/model-c", Enabled: false},
	}); err != nil {
		t.Fatal(err)
	}
	if err := providers.Activate(ctx, providerID); err != nil {
		t.Fatal(err)
	}

	// Simulate a process whose startup config had no model. The endpoint must
	// read the current active provider from SQLite rather than this stale config.
	cfg := config.Defaults()
	cfg.OpenAIModel = ""
	cfg.OpenAIModels = nil
	app := &Handler{Config: &cfg, DB: db}
	response := httptest.NewRecorder()
	app.ListChatModels(response, httptest.NewRequest(http.MethodGet, "/agents/models", nil))

	var result struct {
		Data struct {
			Models []struct {
				BotID   string `json:"bot_id"`
				Name    string `json:"name"`
				Default bool   `json:"default"`
			} `json:"models"`
			DefaultModel string `json:"default_model"`
			Configured bool `json:"configured"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Data.Configured || len(result.Data.Models) != 2 {
		t.Fatalf("expected only the two enabled models from SQLite, got %s", response.Body.String())
	}
	if result.Data.DefaultModel != "provider/model-b" {
		t.Fatalf("expected database default model, got %q", result.Data.DefaultModel)
	}
	cfgForSelection := config.Config{
		ChatProvider: "openai",
		OpenAIModel: result.Data.DefaultModel,
		OpenAIModels: []string{"provider/model-a", "provider/model-b"},
	}
	for _, item := range result.Data.Models {
		modelID, allowed := selectChatModel(&cfgForSelection, item.BotID)
		if !allowed || modelID != item.Name {
			t.Fatalf("model %q was not selectable: model=%q allowed=%v", item.Name, modelID, allowed)
		}
		if item.Default != (item.Name == result.Data.DefaultModel) {
			t.Fatalf("incorrect default flag for %q", item.Name)
		}
	}
}

func TestActiveChatProviderUsesFirstEnabledModelWhenDefaultMissing(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	providers := repository.NewProviderRepository(db)
	providerID, err := providers.Save(ctx, model.Provider{
		Name: "provider-without-default",
		BaseURL: "https://api.example.com/v1",
		TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := providers.ReplaceModels(ctx, providerID, []model.ProviderModel{
		{ProviderID: providerID, ModelID: "first-model", Enabled: true},
		{ProviderID: providerID, ModelID: "second-model", Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := providers.Activate(ctx, providerID); err != nil {
		t.Fatal(err)
	}

	cfg := config.Defaults()
	app := &Handler{Config: &cfg, DB: db}
	active, models, err := app.activeChatProvider(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || active.DefaultModel() != "first-model" {
		t.Fatalf("expected first enabled model as fallback default, config=%#v models=%#v", active, models)
	}
}

func TestListChatModelsReturnsUnconfiguredStateWithoutActiveProvider(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	app := &Handler{Config: &cfg, DB: db}
	response := httptest.NewRecorder()
	app.ListChatModels(response, httptest.NewRequest(http.MethodGet, "/agents/models", nil))
	var result struct {
		Data struct {
			Models []any `json:"models"`
			Configured bool `json:"configured"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || result.Data.Configured || len(result.Data.Models) != 0 {
		t.Fatalf("unexpected empty provider response: HTTP %d %s", response.Code, response.Body.String())
	}
}

func TestActivateProviderChoosesFirstEnabledModelWhenDefaultMissing(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	providers := repository.NewProviderRepository(db)
	providerID, err := providers.Save(ctx, model.Provider{
		Name: "provider-needs-default",
		BaseURL: "https://api.example.com/v1",
		TimeoutSeconds: 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := providers.ReplaceModels(ctx, providerID, []model.ProviderModel{
		{ProviderID: providerID, ModelID: "first-enabled", Enabled: true},
		{ProviderID: providerID, ModelID: "second-enabled", Enabled: true},
	}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	app := &Handler{Config: &cfg, DB: db}
	request := httptest.NewRequest(http.MethodPost, "/admin/providers/activate",
		strings.NewReader(`{"id":`+providerTestID(providerID)+`}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.ActivateProvider(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("activate provider: HTTP %d %s", response.Code, response.Body.String())
	}
	active, models, err := providers.Active(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active.DefaultModel != "first-enabled" || len(models) != 2 {
		t.Fatalf("expected first enabled model to become default, provider=%#v models=%#v", active, models)
	}
}
