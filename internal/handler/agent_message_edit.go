package handler

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

func (h *Handler) PrepareMessageEdit(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	conversationID := strings.TrimSpace(stringValue(payload["conversation_id"]))
	messageID, parseErr := strconv.ParseInt(strings.TrimSpace(stringValue(payload["message_id"])), 10, 64)
	if conversationID == "" || parseErr != nil || messageID < 1 {
		writeError(w, http.StatusBadRequest, "缺少或无效的会话ID、消息ID")
		return
	}
	if !h.Config.UsesLocalConversations() {
		writeError(w, http.StatusNotImplemented, "当前 Coze 会话模式暂不支持编辑历史消息")
		return
	}
	identity, _ := h.identityForRequest(r)
	err = h.Chats.PrepareMessageEdit(r.Context(), messageID, conversationID, identity.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "消息不存在或无权编辑")
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "only user messages") || strings.Contains(err.Error(), "still generating") {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "准备编辑消息失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success"})
}
