package handler

import (
	"database/sql"
	"net/http"
	"strings"
)

func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	if !h.Config.UsesLocalConversations() {
		writeJSON(w, http.StatusOK, map[string]any{
			"code": 0, "msg": "success", "data": []any{},
		})
		return
	}
	identity, _ := currentIdentity(r)
	limit := intValue(r.URL.Query().Get("limit"), 50)
	if limit < 1 || limit > 200 {
		limit = 50
	}
	items, err := h.Chats.ListConversations(r.Context(), identity.Username, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取历史会话失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success", "data": items})
}

func (h *Handler) DeleteConversation(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "请求参数错误")
		return
	}
	id := strings.TrimSpace(stringValue(payload["conversation_id"]))
	if id == "" {
		writeError(w, http.StatusBadRequest, "缺少会话ID")
		return
	}
	if !h.Config.UsesLocalConversations() {
		writeError(w, http.StatusNotImplemented, "当前会话提供方暂不支持本地历史删除")
		return
	}
	identity, _ := currentIdentity(r)
	if err := h.Chats.DeleteConversation(r.Context(), id, identity.Username); err != nil {
		if err == sql.ErrNoRows {
			writeError(w, http.StatusNotFound, "会话不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "删除历史会话失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"code": 0, "msg": "success"})
}
