package handler

import (
	"context"
	"errors"
	"time"

	"pingtai/internal/config"
	"pingtai/internal/model"
	"pingtai/internal/repository"
)

var errNoActiveChatProvider = errors.New("当前没有配置模型渠道，请联系管理员配置")

// activeChatProvider reads the currently activated provider from SQLite for every
// model-list and chat request. Provider changes therefore take effect immediately
// without requiring a process restart.
func (h *Handler) activeChatProvider(ctx context.Context) (config.Config, []model.ProviderModel, error) {
	cfg := *h.Config
	provider, models, err := repository.NewProviderRepository(h.DB).Active(ctx)
	if err != nil {
		return cfg, nil, err
	}
	if len(models) == 0 {
		return cfg, nil, errNoActiveChatProvider
	}

	cfg.ChatProvider = "openai"
	cfg.OpenAIBaseURL = provider.BaseURL
	cfg.OpenAIAPIKey = provider.APIKey
	cfg.OpenAIModel = provider.DefaultModel
	cfg.OpenAIModels = make([]string, 0, len(models))
	cfg.OpenAIMaxTokens = provider.MaxOutputTokens
	cfg.OpenAITimeout = time.Duration(provider.TimeoutSeconds) * time.Second
	if cfg.OpenAITimeout <= 0 {
		cfg.OpenAITimeout = 180 * time.Second
	}
	cfg.OpenAISystemPrompt = provider.SystemPrompt
	cfg.ModelContextTokens = 0

	defaultFound := false
	for _, item := range models {
		cfg.OpenAIModels = append(cfg.OpenAIModels, item.ModelID)
		if item.ModelID != provider.DefaultModel {
			continue
		}
		defaultFound = true
		if item.ContextLength != nil {
			cfg.ModelContextTokens = *item.ContextLength
		}
		if cfg.OpenAIMaxTokens == 0 && item.MaxOutputTokens != nil {
			cfg.OpenAIMaxTokens = int(*item.MaxOutputTokens)
		}
	}

	// If a legacy record has enabled models but no valid default, use the first
	// enabled model instead of treating the whole provider as unconfigured.
	if !defaultFound {
		cfg.OpenAIModel = models[0].ModelID
		cfg.ModelContextTokens = 0
		if cfg.OpenAIMaxTokens == 0 && models[0].MaxOutputTokens != nil {
			cfg.OpenAIMaxTokens = int(*models[0].MaxOutputTokens)
		}
	}
	return cfg, models, nil
}
