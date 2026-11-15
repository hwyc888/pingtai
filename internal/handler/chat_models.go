package handler

import "net/http"

func (h *Handler) ListChatModels(w http.ResponseWriter, r *http.Request) {
	models := h.Config.WebModels()
	defaultModel := h.Config.DefaultModel()
	items := make([]map[string]any, 0, len(models))
	for _, model := range models {
		items = append(items, map[string]any{
			"bot_id": modelBotID(model),
			"name": model,
			"default": model == defaultModel,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]any{
			"models": items,
			"default_model": defaultModel,
			"configured": len(items) > 0,
		},
	})
}
