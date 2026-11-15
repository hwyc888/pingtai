package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pingtai/internal/config"
)

func TestBuiltInChatExposesOnlyAdministratorDefaultModel(t *testing.T) {
	cfg := config.Defaults()
	cfg.OpenAIModel = "provider/model-b"
	cfg.OpenAIModels = []string{"provider/model-a", "provider/model-b", "provider/model-c"}
	app := &Handler{Config: &cfg}
	response := httptest.NewRecorder()
	app.GetAgentsList(response, httptest.NewRequest(http.MethodGet, "/agents/getAgentsList?space_id=openai-workspace", nil))
	var result struct {
		Data struct {
			Bots []struct {
				BotID string `json:"bot_id"`
				Name  string `json:"name"`
			} `json:"space_bots"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Data.Bots) != 1 || result.Data.Bots[0].Name != cfg.OpenAIModel {
		t.Fatalf("web chat should expose only the configured default model: %s", response.Body.String())
	}
	if model, allowed := selectChatModel(&cfg, result.Data.Bots[0].BotID); !allowed || model != cfg.OpenAIModel {
		t.Fatal("the default model was not accepted")
	}
	if _, allowed := selectChatModel(&cfg, modelBotID("provider/model-a")); allowed {
		t.Fatal("a non-default model must not be selectable from built-in web chat")
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
