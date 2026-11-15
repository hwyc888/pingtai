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

// Built-in web chat always uses the administrator-selected default model.
func selectChatModel(cfg *config.Config, botID string) (string, bool) {
	defaultModel := cfg.DefaultModel()
	if cfg.ChatProvider != "openai" || botID == "openai-default" || botID == modelBotID(defaultModel) {
		return defaultModel, true
	}
	return "", false
}
