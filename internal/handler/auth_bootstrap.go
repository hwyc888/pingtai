package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"pingtai/internal/repository"
	"pingtai/internal/service/auth"
)

func (h *Handler) BootstrapStatus(w http.ResponseWriter, r *http.Request) {
	count, err := h.Users.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取账户状态失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]any{"needs_admin": count == 0},
	})
}

func (h *Handler) RegisterFirstAdmin(w http.ResponseWriter, r *http.Request) {
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			!strings.EqualFold(parsed.Host, r.Host) {
			writeError(w, http.StatusForbidden, "首次注册只允许从平台自身页面提交")
			return
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "注册参数格式错误")
		return
	}
	username := strings.TrimSpace(payload.Username)
	if username == "" || utf8.RuneCountInString(username) > 50 {
		writeError(w, http.StatusBadRequest, "账号长度必须为 1-50 个字符")
		return
	}
	if len(payload.Password) < 8 || len(payload.Password) > 72 {
		writeError(w, http.StatusBadRequest, "密码长度必须为 8-72 个字节")
		return
	}
	hash, err := auth.HashPassword(payload.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "密码加密失败")
		return
	}
	_, err = h.Users.CreateFirstAdmin(r.Context(), username, hash)
	if errors.Is(err, repository.ErrBootstrapComplete) {
		writeError(w, http.StatusConflict, "管理员账户已经创建，请返回登录页面")
		return
	}
	if err != nil {
		writeError(w, http.StatusConflict, "创建管理员失败，账号可能已存在")
		return
	}
	pair, err := h.Auth.IssuePair(username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "管理员已创建，请使用刚设置的账号密码登录")
		return
	}
	h.setAuthCookies(w, pair.AccessToken, pair.RefreshToken, "", h.requestSecure(r))
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"msg": "管理员账户创建成功",
		"data": map[string]any{"username": username, "is_admin": true},
	})
}
