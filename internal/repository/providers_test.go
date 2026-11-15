package repository

import (
	"context"
	"testing"

	"pingtai/internal/database"
	"pingtai/internal/model"
)

func TestProviderRepositoryPersistsAndActivatesDefaultProvider(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := NewProviderRepository(db)
	id, err := repo.Save(ctx, model.Provider{
		Name: "test-provider", BaseURL: "https://api.example.com/v1",
		APIKey: "secret-key", DefaultModel: "chat-model",
		MaxOutputTokens: 2048, TimeoutSeconds: 90,
	})
	if err != nil {
		t.Fatalf("save provider: %v", err)
	}
	contextLength := int64(128000)
	outputLength := int64(8192)
	if err := repo.ReplaceModels(ctx, id, []model.ProviderModel{{
		ProviderID: id, ModelID: "chat-model", ContextLength: &contextLength,
		MaxOutputTokens: &outputLength, Enabled: true,
	}}); err != nil {
		t.Fatalf("save provider models: %v", err)
	}
	if err := repo.Activate(ctx, id); err != nil {
		t.Fatalf("activate provider: %v", err)
	}
	active, models, err := repo.Active(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != id || active.APIKey != "secret-key" || active.DefaultModel != "chat-model" {
		t.Fatalf("unexpected active provider: %#v", active)
	}
	if len(models) != 1 || models[0].ModelID != "chat-model" || models[0].ContextLength == nil || *models[0].ContextLength != contextLength {
		t.Fatalf("unexpected active models: %#v", models)
	}
}

func TestProviderRepositoryCanAddManualModelWithoutDiscovery(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := NewProviderRepository(db)
	id, err := repo.Save(ctx, model.Provider{
		Name: "manual-provider", BaseURL: "https://api.example.com/v1",
		DefaultModel: "manual-chat-model", TimeoutSeconds: 90,
	})
	if err != nil {
		t.Fatal(err)
	}
	contextLength := int64(64000)
	if err := repo.UpdateModel(ctx, id, "manual-chat-model", &contextLength, nil, true); err != nil {
		t.Fatalf("insert manual model: %v", err)
	}
	models, err := repo.Models(ctx, id, false)
	if err != nil || len(models) != 1 || models[0].ModelID != "manual-chat-model" || !models[0].Enabled {
		t.Fatalf("manual model was not inserted: models=%#v err=%v", models, err)
	}
	if err := repo.UpdateModel(ctx, id, "manual-chat-model", &contextLength, nil, false); err != nil {
		t.Fatalf("update manual model: %v", err)
	}
	models, err = repo.Models(ctx, id, true)
	if err != nil || len(models) != 0 {
		t.Fatalf("disabled manual model should not be active: models=%#v err=%v", models, err)
	}
}
}
