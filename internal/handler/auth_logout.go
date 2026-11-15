package handler

import (
	"net/http"
)

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	identity, ok := currentIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "msg": "Invalid token"})
		return
	}
	h.Auth.Tokens().Revoke(identity.JTI, identity.Expires)
	writeJSON(w, http.StatusOK, map[string]any{"code": 200, "msg": "退出登录成功"})
}
