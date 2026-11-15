package handler

import (
	"net/http"
)

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	identity, ok := currentIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "msg": "Invalid token"})
		return
	}
	token, err := h.Auth.IssueAccess(identity.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"errcode": 500, "errmsg": "生成访问令牌失败"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"access_token": token, "errcode": 200, "errmsg": "success"})
}
