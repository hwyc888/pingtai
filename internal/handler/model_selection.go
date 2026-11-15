package handler

import (
	"crypto/sha256"
	"fmt"

	"pingtai/internal/config"
)

// Fixed-length IDs fit the existing bot_id database column even for long provider model names.
func modelBotID(model string) string {
	return fmt.Sprintf("openai-model-%x", sha256.Sum256([]byte(model)))
}

func selectChatModel(cfg *config.Config, botID string) (string, bool) {
	if cfg.ChatProvider != "openai" {
		return "", false
	}
	models := cfg.WebModels()
	if len(models) == 0 {
		return "", false
	}
	if botID == "" || botID == "openai-default" {
		return cfg.DefaultModel(), cfg.DefaultModel() != ""
	}
	for _, model := range models {
		if botID == modelBotID(model) {
			return model, true
		}
	}
	return "", false
}
