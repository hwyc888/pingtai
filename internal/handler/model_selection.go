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
	if cfg.ChatProvider != "openai" || botID == "openai-default" {
		return cfg.DefaultModel(), true
	}
	for _, model := range cfg.WebModels() {
		if modelBotID(model) == botID {
			return model, true
		}
	}
	return "", false
}
