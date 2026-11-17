package handler

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"pingtai/internal/service/auth"
)

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.Config.LocalLoginEnabled {
		writeJSON(w, http.StatusForbidden, map[string]any{"errcode": 403, "errmsg": "平台账户密码登录未启用，请返回登录方式选择页面"})
		return
	}
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
	secure := h.requestSecure(r)
	h.setAuthCookies(w, pair.AccessToken, pair.RefreshToken, "", "local", secure)
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
