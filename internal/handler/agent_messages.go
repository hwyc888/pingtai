package handler

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (h *Handler) CreateMessage(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "请求参数错误"})
		return
	}
	conversationID := strings.TrimSpace(stringValue(payload["conversation_id"]))
	content := stringValue(payload["content"])
	role := defaultString(stringValue(payload["role"]), "user")
	if conversationID == "" || content == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少会话ID或消息内容"})
		return
	}
	if h.Config.UsesLocalConversations() {
		identity, _ := currentIdentity(r)
		if _, err := h.Chats.FindConversation(r.Context(), conversationID, identity.Username); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"code": 404, "msg": "会话不存在"})
			return
		}
		message, err := h.Chats.InsertMessage(r.Context(), conversationID, "", role, content, "")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "保存消息失败")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": map[string]any{"id": message.ID}})
		return
	}
	query := url.Values{"conversation_id": []string{conversationID}}
	h.callCozeJSON(w, r, http.MethodPost, "/v1/conversation/message/create", query, payload, nil)
}

func (h *Handler) GetMessageList(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "请求参数错误"})
		return
	}
	conversationID := strings.TrimSpace(stringValue(payload["conversation_id"]))
	if conversationID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少会话ID参数"})
		return
	}
	order := strings.ToLower(defaultString(stringValue(payload["order"]), "desc"))
	limit := intValue(payload["limit"], 50)
	if limit < 1 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if h.Config.UsesLocalConversations() {
		identity, _ := currentIdentity(r)
		if _, err := h.Chats.FindConversation(r.Context(), conversationID, identity.Username); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"code": 404, "msg": "会话不存在"})
			return
		}
		items, err := h.Chats.ListMessages(r.Context(), conversationID, limit, order)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "读取消息列表失败")
			return
		}
		messages := make([]map[string]any, 0, len(items))
		for _, item := range items {
			messages = append(messages, formatMessage(item))
		}
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": messages})
		return
	}
	query := url.Values{"conversation_id": []string{conversationID}}
	h.callCozeJSON(w, r, http.MethodPost, "/v1/conversation/message/list", query,
		map[string]any{"order": order, "limit": limit}, nil)
}

func (h *Handler) GetMessage(w http.ResponseWriter, r *http.Request) {
	conversationID := strings.TrimSpace(r.URL.Query().Get("conversation_id"))
	messageID := strings.TrimSpace(r.URL.Query().Get("message_id"))
	if conversationID == "" || messageID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "缺少会话ID或消息ID参数"})
		return
	}
	if h.Config.UsesLocalConversations() {
		identity, _ := currentIdentity(r)
		id, err := strconv.ParseInt(messageID, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"code": 500, "msg": "消息ID格式错误"})
			return
		}
		item, err := h.Chats.FindMessage(r.Context(), id, conversationID, identity.Username)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"code": 404, "msg": "消息不存在"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": formatMessage(item)})
		return
	}
	query := url.Values{"conversation_id": []string{conversationID}, "message_id": []string{messageID}}
	h.callCozeJSON(w, r, http.MethodGet, "/v1/conversation/message/retrieve", query, nil, nil)
}
