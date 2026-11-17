package handler

import "net/http"

func (h *Handler) AuthMode(w http.ResponseWriter, r *http.Request) {
	mode := h.Config.LoginMode()
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]any{
			"mode": mode,
			"local_login_enabled": h.Config.LocalLoginEnabled,
			"sso_login_enabled": h.Config.SSOConfigured(),
		},
	})
}
