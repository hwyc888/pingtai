package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

func (h *Handler) ClearConversation(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	conversationID := strings.TrimSpace(stringValue(payload["conversation_id"]))
	if conversationID == "" {
		writeError(w, http.StatusBadRequest, "缺少会话ID")
		return
	}
	if !h.Config.UsesLocalConversations() {
		writeError(w, http.StatusNotImplemented, "当前会话提供方暂不支持清空本地会话")
		return
	}
	identity, _ := h.identityForRequest(r)
	if err := h.Chats.ClearConversation(r.Context(), conversationID, identity.UserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "会话不存在或无权清空")
			return
		}
		writeError(w, http.StatusInternalServerError, "清空当前会话失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success"})
}

func (h *Handler) ClearAllConversations(w http.ResponseWriter, r *http.Request) {
	if !h.Config.UsesLocalConversations() {
		writeError(w, http.StatusNotImplemented, "当前会话提供方暂不支持清空本地历史")
		return
	}
	identity, _ := h.identityForRequest(r)
	deleted, err := h.Chats.ClearAllConversations(r.Context(), identity.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "清空历史对话失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"msg": "success",
		"data": map[string]any{"deleted_count": deleted},
	})
}
