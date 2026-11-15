package handler

import (
	"net/http"
)

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if identity, ok := h.ssoIdentityFromRequest(r); ok {
		h.Auth.Tokens().Revoke(identity.JTI, identity.Expires)
	}
	if cookie, err := r.Cookie("RefreshToken"); err == nil {
		if claims, parseErr := h.Auth.Parse(cookie.Value); parseErr == nil && claims.Type == "refresh" {
			h.Auth.Tokens().Revoke(claims.JTI, claims.Expires)
		}
	}
	clearAuthCookies(w, h.requestSecure(r))
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"code": 200, "msg": "退出登录成功"})
}

