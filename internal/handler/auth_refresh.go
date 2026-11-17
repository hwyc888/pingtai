package handler

import (
	"context"
	"net/http"
	"time"

	"pingtai/internal/service/sso"
)

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identityForRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "msg": "Invalid token"})
		return
	}
	if identity.Provider == "sso" {
		providerCookie, cookieErr := r.Cookie("SSORefreshToken")
		if cookieErr != nil {
			clearAuthCookies(w, h.requestSecure(r))
			writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "msg": "学校统一身份认证已过期，请重新登录"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		providerToken, refreshErr := sso.NewClient(*h.Config).RefreshToken(ctx, providerCookie.Value)
		cancel()
		if refreshErr != nil {
			clearAuthCookies(w, h.requestSecure(r))
			writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "msg": "学校统一身份认证已过期，请重新登录"})
			return
		}
		if providerToken.RefreshToken != "" {
			refreshTTL := h.Config.RefreshTokenTTL
			if refreshTTL <= 0 {
				refreshTTL = 30 * 24 * time.Hour
			}
			http.SetCookie(w, &http.Cookie{
				Name: "SSORefreshToken", Value: providerToken.RefreshToken, Path: "/",
				Expires: time.Now().Add(refreshTTL), MaxAge: int(refreshTTL.Seconds()),
				HttpOnly: true, Secure: h.requestSecure(r), SameSite: http.SameSiteLaxMode,
			})
		}
	}
	token, err := h.Auth.IssueAccessForIdentity(identity.Username, identity.DisplayName, identity.UserID, identity.Provider)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"errcode": 500, "errmsg": "生成访问令牌失败"})
		return
	}
	accessTTL := h.Config.AccessTokenTTL
	if accessTTL <= 0 {
		accessTTL = 30 * time.Minute
	}
	http.SetCookie(w, &http.Cookie{
		Name: "Authorization", Value: token, Path: "/",
		Expires: time.Now().Add(accessTTL), MaxAge: int(accessTTL.Seconds()),
		HttpOnly: true, Secure: h.requestSecure(r), SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"access_token": token, "errcode": 200, "errmsg": "success"})
}
