package handler

import (
	"net/http"
	"net/url"
	"strings"

	"pingtai/internal/util"
)

func (h *Handler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	payload := map[string]any{}
	if r.ContentLength > 0 {
		parsed, err := readJSONMap(r)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "请求参数错误"})
			return
		}
		payload = parsed
	}
	botID := stringValue(payload["bot_id"])
	if h.Config.UsesLocalConversations() {
		identity, _ := h.identityForRequest(r)
		id, err := util.NewID()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "生成会话ID失败")
			return
		}
		if err := h.Chats.CreateConversation(r.Context(), id, identity.UserID, botID); err != nil {
			writeError(w, http.StatusInternalServerError, "创建会话失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"code": 0, "msg": "success",
			"data": map[string]any{"id": id},
		})
		return
	}
	h.callCozeJSON(w, r, http.MethodPost, "/v1/conversation/create", nil, payload, nil)
}

func (h *Handler) GetConversation(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimSpace(r.URL.Query().Get("conversation_id"))
	if conversationID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少会话ID参数"})
		return
	}
	if h.Config.UsesLocalConversations() {
		identity, _ := h.identityForRequest(r)
		item, err := h.Chats.FindConversation(r.Context(), conversationID, identity.UserID)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"code": 404, "msg": "会话不存在"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": item})
		return
	}
	query := url.Values{"conversation_id": []string{conversationID}}
	h.callCozeJSON(w, r, http.MethodGet, "/v1/conversation/retrieve", query, nil, nil)
}
