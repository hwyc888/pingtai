package handler

import (
	"net/http"
	"net/url"
	"strings"

	"pingtai/internal/config"
)

func (h *Handler) GetAuthSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"data": map[string]any{
			"mode": h.Config.AuthMode,
			"sso_base_url": h.Config.SSOBaseURL,
			"sso_service_url": h.Config.SSOServiceURL,
			"sso_client_id": h.Config.SSOClientID,
			"sso_redirect_url": h.Config.SSORedirectURL,
			"sso_admin_accounts": h.Config.SSOAdminAccounts,
			"has_client_secret": strings.TrimSpace(h.Config.SSOClientSecret) != "",
		},
	})
}

func (h *Handler) SaveAuthSettings(w http.ResponseWriter, r *http.Request) {
	payload, err := readJSONMap(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "登录设置参数格式错误")
		return
	}
	next := *h.Config
	if value := strings.TrimSpace(stringValue(payload["mode"])); value != "" {
		next.AuthMode = strings.ToLower(value)
	}
	if value := strings.TrimSpace(stringValue(payload["sso_base_url"])); value != "" {
		next.SSOBaseURL = strings.TrimRight(value, "/")
	}
	if value := strings.TrimSpace(stringValue(payload["sso_service_url"])); value != "" {
		next.SSOServiceURL = value
	}
	if value := strings.TrimSpace(stringValue(payload["sso_client_id"])); value != "" {
		next.SSOClientID = value
	}
	if value := strings.TrimSpace(stringValue(payload["sso_client_secret"])); value != "" {
		next.SSOClientSecret = value
	}
	if value := strings.TrimSpace(stringValue(payload["sso_redirect_url"])); value != "" {
		next.SSORedirectURL = value
	}
	if value, exists := payload["sso_admin_accounts"]; exists {
		next.SSOAdminAccounts = strings.TrimSpace(stringValue(value))
	}
	if next.SSOBaseURL == "" {
		next.SSOBaseURL = "https://ids.zcvc.cn/cas"
	}
	if next.AuthMode != "local" && next.AuthMode != "sso" {
		writeError(w, http.StatusBadRequest, "登录方式必须是账户登录或学校统一身份认证")
		return
	}
	if next.AuthMode == "sso" {
		if !next.SSOSettingsComplete() {
			writeError(w, http.StatusBadRequest, "请完整填写 CAS 地址、Client ID、Client Secret 和回调地址")
			return
		}
		if strings.TrimSpace(next.SSOAdminAccounts) == "" {
			writeError(w, http.StatusBadRequest, "请至少配置一个学校管理员账号，多个账号可用逗号分隔")
			return
		}
		base, parseErr := url.Parse(next.SSOBaseURL)
		if parseErr != nil || base.Hostname() == "" || (base.Scheme != "https" && base.Scheme != "http") {
			writeError(w, http.StatusBadRequest, "CAS 服务地址必须是有效的 HTTP 或 HTTPS 地址")
			return
		}
		redirect, parseErr := url.Parse(next.SSORedirectURL)
		if parseErr != nil || redirect.Hostname() == "" || (redirect.Scheme != "https" && redirect.Scheme != "http") {
			writeError(w, http.StatusBadRequest, "回调地址必须是有效的 HTTP 或 HTTPS 地址")
			return
		}
		if service := strings.TrimSpace(next.SSOServiceURL); service != "" {
			parsed, serviceErr := url.Parse(service)
			if serviceErr != nil || parsed.Hostname() == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
				writeError(w, http.StatusBadRequest, "应用访问地址必须是有效的 HTTP 或 HTTPS 地址")
				return
			}
		}
	}
	if err := next.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := config.SaveAuthSettings(r.Context(), h.DB, next); err != nil {
		writeError(w, http.StatusInternalServerError, "保存登录设置失败")
		return
	}
	*h.Config = next
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0,
		"msg": "登录设置已保存",
		"data": map[string]any{
			"mode": next.AuthMode,
			"has_client_secret": strings.TrimSpace(next.SSOClientSecret) != "",
		},
	})
}
