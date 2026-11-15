package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pingtai/internal/config"
)

func TestSelectedModelsAppearAsDistinctChatOptions(t *testing.T) {
	cfg := config.Defaults()
	cfg.OpenAIModel = "provider/model-a"
	cfg.OpenAIModels = []string{cfg.OpenAIModel, "provider/model-b", strings.Repeat("long-name-", 30)}
	handler := &Handler{Config: &cfg}
	response := httptest.NewRecorder()
	handler.GetAgentsList(response, httptest.NewRequest(http.MethodGet, "/agents/getAgentsList?space_id=openai-workspace", nil))
	var result struct {
		Data struct {
			Bots []struct {
				BotID string `json:"bot_id"`
				Name  string `json:"name"`
			} `json:"space_bots"`
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || len(result.Data.Bots) != 3 {
		t.Fatalf("unexpected model options: %s", response.Body.String())
	}
	for index, bot := range result.Data.Bots {
		model, allowed := selectChatModel(&cfg, bot.BotID)
		if !allowed || model != cfg.OpenAIModels[index] || bot.Name != model || len(bot.BotID) > 128 {
			t.Fatalf("model option did not resolve correctly: %#v", bot)
		}
	}
	if model, allowed := selectChatModel(&cfg, "openai-default"); !allowed || model != cfg.OpenAIModel {
		t.Fatal("legacy default option stopped working")
	}
	if _, allowed := selectChatModel(&cfg, modelBotID("not-selected")); allowed {
		t.Fatal("an unselected web model was accepted")
	}
	cfg.OpenAIModel = "provider/model-b"
	if models := cfg.WebModels(); models[0] != cfg.OpenAIModel || cfg.OpenAIModels[0] != "provider/model-a" {
		t.Fatal("default should be listed first without mutating saved model order")
	}
	cfg.OpenAIModels = nil
	if model, allowed := selectChatModel(&cfg, modelBotID(cfg.OpenAIModel)); !allowed || model != cfg.OpenAIModel {
		t.Fatal("single-model configuration stopped working")
	}
}
