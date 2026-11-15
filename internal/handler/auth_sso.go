package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pingtai/internal/middleware"
	"pingtai/internal/service/sso"
)

const ssoStateCookie = "pingtai_sso_state"
const ssoReturnCookie = "pingtai_sso_return_to"
const signedInCookie = "pingtai_signed_in"

func (h *Handler) SSOLogin(w http.ResponseWriter, r *http.Request) {
	if !h.Config.SSOConfigured() {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "msg": "无法初始化统一身份认证"})
		return
	}
	state := base64.RawURLEncoding.EncodeToString(random[:])
	returnTo := safeSSOReturnTo(r.URL.Query().Get("return_to"))
	http.SetCookie(w, &http.Cookie{
		Name: ssoReturnCookie, Value: returnTo, Path: "/auth/sso/callback",
		MaxAge: 600, Expires: time.Now().Add(10 * time.Minute),
		HttpOnly: true, Secure: h.ssoSecure(), SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: ssoStateCookie, Value: state, Path: "/auth/sso/callback",
		MaxAge: 600, Expires: time.Now().Add(10 * time.Minute),
		HttpOnly: true, Secure: h.ssoSecure(), SameSite: http.SameSiteLaxMode,
	})
	target, err := sso.NewClient(*h.Config).AuthorizationURL(state)
	if err != nil {
		clearSSOCookies(w, h.ssoSecure())
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "msg": "无法生成统一身份认证地址"})
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handler) SSOCallback(w http.ResponseWriter, r *http.Request) {
	returnTo := "/IntelligentAI"
	if cookie, err := r.Cookie(ssoReturnCookie); err == nil {
		returnTo = safeSSOReturnTo(cookie.Value)
	}
	if providerError := r.URL.Query().Get("error"); providerError != "" {
		clearSSOCookies(w, h.ssoSecure())
		if returnTo == "/setup" || returnTo == "/setup/" {
			http.Redirect(w, r, "/setup?sso_error=1", http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/login?sso_error=1", http.StatusSeeOther)
		}
		return
	}
	stateCookie, err := r.Cookie(ssoStateCookie)
	state := r.URL.Query().Get("state")
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(state)) != 1 {
		clearSSOCookies(w, h.ssoSecure())
		http.Redirect(w, r, "/login?sso_error=1", http.StatusSeeOther)
		return
	}
	clearSSOCookies(w, h.ssoSecure())

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" || !h.Config.SSOConfigured() {
		http.Redirect(w, r, "/login?sso_error=1", http.StatusSeeOther)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	client := sso.NewClient(*h.Config)
	providerToken, err := client.ExchangeCode(ctx, code)
	if err != nil {
		http.Redirect(w, r, "/login?sso_error=1", http.StatusSeeOther)
		return
	}
	profile, err := client.UserProfile(ctx, providerToken.AccessToken)
	if err != nil {
		http.Redirect(w, r, "/login?sso_error=1", http.StatusSeeOther)
		return
	}
	pair, err := h.Auth.IssuePairWithName(profile.Account, profile.DisplayName)
	if err != nil {
		http.Redirect(w, r, "/login?sso_error=1", http.StatusSeeOther)
		return
	}
	h.setAuthCookies(w, pair.AccessToken, pair.RefreshToken, providerToken.RefreshToken)
	http.Redirect(w, r, returnTo, http.StatusSeeOther)
}

func (h *Handler) CurrentUser(w http.ResponseWriter, r *http.Request) {
	identity, ok := currentIdentity(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "msg": "登录状态已失效"})
		return
	}
	displayName := identity.DisplayName
	if displayName == "" {
		displayName = identity.Username
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0, "msg": "success",
		"data": map[string]any{
			"account": identity.Username,
			"display_name": displayName,
			"is_admin": h.Config.IsSSOAdmin(identity.Username),
		},
	})
}

func (h *Handler) ssoSecure() bool {
	redirect, err := url.Parse(h.Config.SSORedirectURL)
	return err == nil && strings.EqualFold(redirect.Scheme, "https")
}

func (h *Handler) setAuthCookies(w http.ResponseWriter, accessToken, refreshToken, providerRefreshToken string) {
	secure := h.ssoSecure()
	now := time.Now()
	accessTTL := h.Config.AccessTokenTTL
	if accessTTL <= 0 {
		accessTTL = 30 * time.Minute
	}
	refreshTTL := h.Config.RefreshTokenTTL
	if refreshTTL <= 0 {
		refreshTTL = 30 * 24 * time.Hour
	}
	http.SetCookie(w, &http.Cookie{
		Name: "Authorization", Value: accessToken, Path: "/",
		Expires: now.Add(accessTTL), MaxAge: int(accessTTL.Seconds()),
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: "RefreshToken", Value: refreshToken, Path: "/",
		Expires: now.Add(refreshTTL), MaxAge: int(refreshTTL.Seconds()),
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	// This non-secret flag is only used by the SPA route guard. API authorization
	// always validates the HttpOnly signed tokens on the server.
	if providerRefreshToken != "" {
		http.SetCookie(w, &http.Cookie{
			Name: "SSORefreshToken", Value: providerRefreshToken, Path: "/",
			Expires: now.Add(refreshTTL), MaxAge: int(refreshTTL.Seconds()),
			HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		})
	}
	http.SetCookie(w, &http.Cookie{
		Name: signedInCookie, Value: "1", Path: "/",
		Expires: now.Add(refreshTTL), MaxAge: int(refreshTTL.Seconds()),
		Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

func clearAuthCookies(w http.ResponseWriter, secure bool) {
	for _, name := range []string{"Authorization", "RefreshToken", "SSORefreshToken", signedInCookie} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/", MaxAge: -1,
			Expires: time.Unix(1, 0), HttpOnly: name != signedInCookie,
			Secure: secure, SameSite: http.SameSiteLaxMode,
		})
	}
}

func clearSSOCookies(w http.ResponseWriter, secure bool) {
	for _, name := range []string{ssoStateCookie, ssoReturnCookie} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/auth/sso/callback",
			MaxAge: -1, Expires: time.Unix(1, 0),
			HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		})
	}
}

func safeSSOReturnTo(value string) string {
	switch value {
	case "/setup", "/setup/":
		return value
	default:
		return "/IntelligentAI"
	}
}

func (h *Handler) ssoIdentityFromRequest(r *http.Request) (middleware.Identity, bool) {
	token := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(token), "bearer ") {
		token = strings.TrimSpace(token[len("Bearer "):])
	}
	if token == "" {
		cookie, err := r.Cookie("Authorization")
		if err != nil {
			return middleware.Identity{}, false
		}
		token = cookie.Value
	}
	claims, err := h.Auth.Parse(token)
	if err != nil || claims.Type != "access" {
		return middleware.Identity{}, false
	}
	return middleware.Identity{
		Username: claims.Subject, DisplayName: claims.Name, JTI: claims.JTI, Expires: claims.Expires,
	}, true
}
