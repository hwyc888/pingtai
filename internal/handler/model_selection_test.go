package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pingtai/internal/config"
)

func TestBuiltInChatExposesEnabledModelsAndAllowsSelection(t *testing.T) {
	cfg := config.Defaults()
	cfg.OpenAIModel = "provider/model-b"
	cfg.OpenAIModels = []string{"provider/model-a", "provider/model-b", "provider/model-c"}
	app := &Handler{Config: &cfg}
	response := httptest.NewRecorder()
	app.ListChatModels(response, httptest.NewRequest(http.MethodGet, "/agents/models", nil))
	var result struct {
		Data struct {
			Models []struct {
				BotID   string `json:"bot_id"`
				Name    string `json:"name"`
				Default bool   `json:"default"`
			} `json:"models"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data.Models) != 3 {
		t.Fatalf("expected all three enabled models, got %s", response.Body.String())
	}
	for _, item := range result.Data.Models {
		model, allowed := selectChatModel(&cfg, item.BotID)
		if !allowed || model != item.Name {
			t.Fatalf("model %q was not selectable: model=%q allowed=%v", item.Name, model, allowed)
		}
		if item.Default != (item.Name == cfg.OpenAIModel) {
			t.Fatalf("incorrect default flag for %q", item.Name)
		}
	}
	if model, allowed := selectChatModel(&cfg, "openai-default"); !allowed || model != cfg.OpenAIModel {
		t.Fatal("legacy default bot ID should remain compatible")
	}
	if models := cfg.WebModels(); models[0] != cfg.OpenAIModel || cfg.OpenAIModels[0] != "provider/model-a" {
		t.Fatal("default should be listed first without mutating saved model order")
	}
	cfg.OpenAIModels = nil
	if model, allowed := selectChatModel(&cfg, modelBotID(cfg.OpenAIModel)); !allowed || model != cfg.OpenAIModel {
		t.Fatal("single-model configuration stopped working")
	}
}
