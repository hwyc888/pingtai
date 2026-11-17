package handler

import (
	"net/http"
	"net/url"
	"strings"
)

func (h *Handler) GetChatDetail(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimSpace(r.URL.Query().Get("conversation_id"))
	chatID := strings.TrimSpace(r.URL.Query().Get("chat_id"))
	if conversationID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少会话ID参数"})
		return
	}
	if chatID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少聊天ID参数"})
		return
	}
	if h.Config.UsesLocalConversations() {
		identity, _ := h.identityForRequest(r)
		item, err := h.Chats.FindChat(r.Context(), chatID, conversationID, identity.UserID)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"code": 404, "msg": "聊天记录不存在"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"code": 0, "msg": "success",
			"data": map[string]any{"id": item.ID, "conversation_id": item.ConversationID, "status": item.Status, "last_error": item.ErrorMessage},
		})
		return
	}
	query := url.Values{"conversation_id": []string{conversationID}, "chat_id": []string{chatID}}
	h.callCozeJSON(w, r, http.MethodGet, "/v3/chat/retrieve", query, nil, nil)
}
