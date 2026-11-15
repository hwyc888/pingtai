package handler

import (
	"net/http"
	"net/url"
	"strings"
)

func (h *Handler) GetChatMessage(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimSpace(r.URL.Query().Get("conversation_id"))
	chatID := strings.TrimSpace(r.URL.Query().Get("chat_id"))
	if conversationID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少会话ID参数"})
		return
	}
	if chatID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少消息ID参数"})
		return
	}
	if h.Config.UsesLocalConversations() {
		identity, _ := currentIdentity(r)
		items, err := h.Chats.MessagesByChat(r.Context(), chatID, conversationID, identity.Username)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"code": 404, "msg": "聊天消息不存在"})
			return
		}
		messages := make([]map[string]any, 0, len(items))
		for _, item := range items {
			messages = append(messages, formatMessage(item))
		}
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": messages})
		return
	}
	query := url.Values{"conversation_id": []string{conversationID}, "chat_id": []string{chatID}}
	h.callCozeJSON(w, r, http.MethodGet, "/v3/chat/message/list", query, nil, nil)
}
