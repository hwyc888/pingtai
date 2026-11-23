package handler

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"log"
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
		log.Printf("[sso] login blocked: SSO settings are incomplete")
		http.Redirect(w, r, "/login?sso_error=config", http.StatusSeeOther)
		return
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		log.Printf("[sso] cannot generate login state: %v", err)
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
		log.Printf("[sso] cannot build authorization URL: %v", err)
		clearSSOCookies(w, h.ssoSecure())
		http.Redirect(w, r, "/login?sso_error=config", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (h *Handler) SSOCallback(w http.ResponseWriter, r *http.Request) {
	returnTo := "/IntelligentAI"
	if cookie, err := r.Cookie(ssoReturnCookie); err == nil {
		returnTo = safeSSOReturnTo(cookie.Value)
	}
	if providerError := strings.TrimSpace(r.URL.Query().Get("error")); providerError != "" {
		log.Printf("[sso] authorization provider returned error=%s", safeSSOErrorCode(providerError))
		clearSSOCookies(w, h.ssoSecure())
		http.Redirect(w, r, "/login?sso_error=provider", http.StatusSeeOther)
		return
	}
	stateCookie, err := r.Cookie(ssoStateCookie)
	state := r.URL.Query().Get("state")
	if err != nil || state == "" || subtle.ConstantTimeCompare([]byte(stateCookie.Value), []byte(state)) != 1 {
		log.Printf("[sso] callback state validation failed: state_cookie_present=%t state_parameter_present=%t request_host=%q configured_callback_host=%q",
			err == nil, state != "", r.Host, ssoCallbackHost(h.Config.SSORedirectURL))
		clearSSOCookies(w, h.ssoSecure())
		http.Redirect(w, r, "/login?sso_error=state", http.StatusSeeOther)
		return
	}
	clearSSOCookies(w, h.ssoSecure())

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		log.Printf("[sso] callback did not contain an authorization code")
		http.Redirect(w, r, "/login?sso_error=missing_code", http.StatusSeeOther)
		return
	}
	if !h.Config.SSOConfigured() {
		log.Printf("[sso] callback received a code but SSO settings are incomplete")
		http.Redirect(w, r, "/login?sso_error=config", http.StatusSeeOther)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	client := sso.NewClient(*h.Config)
	providerToken, err := client.ExchangeCode(ctx, code)
	if err != nil {
		log.Printf("[sso] authorization-code exchange failed: %v", err)
		http.Redirect(w, r, "/login?sso_error=token", http.StatusSeeOther)
		return
	}
	profile, err := client.UserProfile(ctx, providerToken.AccessToken)
	if err != nil {
		log.Printf("[sso] user-profile request failed: %v", err)
		http.Redirect(w, r, "/login?sso_error=profile", http.StatusSeeOther)
		return
	}
	stableIdentity, err := h.Users.EnsureExternalIdentity(ctx, "sso", profile.Account, profile.Account, profile.DisplayName)
	if err != nil {
		log.Printf("[sso] cannot persist external identity: %v", err)
		http.Redirect(w, r, "/login?sso_error=identity", http.StatusSeeOther)
		return
	}
	pair, err := h.Auth.IssuePairForIdentity(profile.Account, profile.DisplayName, stableIdentity.ID, "sso")
	if err != nil {
		log.Printf("[sso] cannot issue platform session: %v", err)
		http.Redirect(w, r, "/login?sso_error=session", http.StatusSeeOther)
		return
	}
	h.setAuthCookies(w, pair.AccessToken, pair.RefreshToken, providerToken.RefreshToken, "sso", h.ssoSecure() || r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https"))
	http.Redirect(w, r, returnTo, http.StatusSeeOther)
}

func ssoCallbackHost(callbackURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(callbackURL))
	if err != nil {
		return ""
	}
	return parsed.Host
}

func safeSSOErrorCode(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 48 {
		value = value[:48]
	}
	var builder strings.Builder
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' {
			builder.WriteRune(character)
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}

func (h *Handler) SSOLogout(w http.ResponseWriter, r *http.Request) {
	clearAuthCookies(w, h.requestSecure(r))
	w.Header().Set("Cache-Control", "no-store")
	baseURL := strings.TrimRight(strings.TrimSpace(h.Config.SSOBaseURL), "/")
	if baseURL == "" {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	target := baseURL + "/logout"
	if service := strings.TrimSpace(h.Config.SSOServiceURL); service != "" {
		query := url.Values{}
		query.Set("service", service)
		target += "?" + query.Encode()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (h *Handler) CurrentUser(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.identityForRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"code": 401, "msg": "登录状态已失效"})
		return
	}
	displayName := identity.DisplayName
	if displayName == "" {
		displayName = identity.Username
	}
	authMethod := "local"
	if cookie, err := r.Cookie("pingtai_auth_method"); err == nil && cookie.Value == "sso" {
		authMethod = "sso"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": 0, "msg": "success",
		"data": map[string]any{
			"account": identity.Username,
			"display_name": displayName,
			"user_id": identity.UserID,
			"auth_provider": identity.Provider,
			"auth_method": authMethod,
			"is_admin": h.isAdministrator(r, identity.Username),
		},
	})
}


func (h *Handler) isAdministrator(r *http.Request, account string) bool {
	identity, ok := h.identityForRequest(r)
	if ok && identity.Provider == "sso" {
		return h.Config.IsSSOAdmin(identity.Username)
	}
	if h.Users == nil {
		return false
	}
	user, err := h.Users.FindByUsername(r.Context(), account)
	return err == nil && user.UserType == 1 && user.Status == 1 && user.DelTag == 0
}

func (h *Handler) requestSecure(r *http.Request) bool {
	return h.ssoSecure() || r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func (h *Handler) ssoSecure() bool {
	redirect, err := url.Parse(h.Config.SSORedirectURL)
	return err == nil && strings.EqualFold(redirect.Scheme, "https")
}

func (h *Handler) setAuthCookies(w http.ResponseWriter, accessToken, refreshToken, providerRefreshToken, authMethod string, secure bool) {
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
	} else {
		http.SetCookie(w, &http.Cookie{
			Name: "SSORefreshToken", Value: "", Path: "/", MaxAge: -1,
			Expires: time.Unix(1, 0), HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		})
	}
	http.SetCookie(w, &http.Cookie{
		Name: signedInCookie, Value: "1", Path: "/",
		Expires: now.Add(refreshTTL), MaxAge: int(refreshTTL.Seconds()),
		Secure: secure, SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name: "pingtai_auth_method", Value: authMethod, Path: "/",
		Expires: now.Add(refreshTTL), MaxAge: int(refreshTTL.Seconds()),
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

func clearAuthCookies(w http.ResponseWriter, secure bool) {
	for _, name := range []string{"Authorization", "RefreshToken", "SSORefreshToken", "pingtai_auth_method", signedInCookie} {
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
	if value == "/IntelligentAI" {
		return value
	}
	return "/IntelligentAI"
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
		Username: claims.Subject, DisplayName: claims.Name, UserID: claims.UserID, Provider: legacyAuthProvider(r, claims.Provider), JTI: claims.JTI, Expires: claims.Expires,
	}, true
}

// Legacy tokens did not include a provider claim. Use the existing login-method cookie only for that case.
func legacyAuthProvider(r *http.Request, provider string) string {
	if provider != "" {
		return provider
	}
	if cookie, err := r.Cookie("pingtai_auth_method"); err == nil && cookie.Value == "sso" {
		return "sso"
	}
	return "local"
}
