package handler

import (
	"database/sql"
	"errors"
	"net/http"
)

func (h *Handler) ListChatModels(w http.ResponseWriter, r *http.Request) {
	cfg, providerModels, err := h.activeChatProvider(r.Context())
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, errNoActiveChatProvider) {
		writeJSON(w, http.StatusOK, map[string]any{
			"code": 0,
			"data": map[string]any{
				"models": []any{},
				"default_model": "",
				"configured": false,
				"supports_message_edit": h.Config.UsesLocalConversations(),
				"supports_local_history": h.Config.UsesLocalConversations(),
			},
		})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取当前模型供应商失败")
		return
	}

	defaultModel := cfg.DefaultModel()
	items := make([]map[string]any, 0, len(providerModels))
	for _, item := range providerModels {
		items = append(items, map[string]any{
			"bot_id": modelBotID(item.ModelID),
			"name": item.ModelID,
			"default": item.ModelID == defaultModel,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]any{
			"models": items,
			"default_model": defaultModel,
			"configured": len(items) > 0,
			"supports_message_edit": h.Config.UsesLocalConversations(),
				"supports_local_history": h.Config.UsesLocalConversations(),
		},
	})
}
