package handler

import (
	"net/http"
)

func (h *Handler) AuthMode(w http.ResponseWriter, r *http.Request) {
	mode := "local"
	if h.Config.SSOConfigured() {
		mode = "sso"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]string{"mode": mode},
	})
}
