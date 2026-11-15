package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"pingtai/internal/service/auth"
)

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.parseForm(w, r) {
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	if username == "" || password == "" {
		writeJSON(w, http.StatusOK, map[string]any{"errcode": 401, "errmsg": "用户名或密码为空"})
		return
	}
	user, err := h.Auth.Authenticate(r.Context(), username, password)
	if err != nil {
		if err == sql.ErrNoRows || err == auth.ErrInvalidCredentials {
			writeJSON(w, http.StatusOK, map[string]any{"errcode": 401, "errmsg": "用户名或密码错误"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"errcode": 500, "errmsg": "登录失败"})
		return
	}
	pair, err := h.Auth.IssuePair(user.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"errcode": 500, "errmsg": "生成登录令牌失败"})
		return
	}
	secure := h.ssoSecure() || r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
	h.setAuthCookies(w, pair.AccessToken, pair.RefreshToken, "", secure)
	bindWX := 0
	if user.OpenID != "" {
		bindWX = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":     pair.AccessToken,
		"refresh_token":    pair.RefreshToken,
		"user_type":        user.UserType,
		"token_expiration": time.Now().Add(30 * time.Minute).Unix(),
		"created_at":       user.CreatedAt,
		"bind_wx":          bindWX,
		"errcode":          200,
		"errmsg":           "登录成功",
	})
}
