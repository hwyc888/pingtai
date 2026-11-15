package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pingtai/internal/config"
	authservice "pingtai/internal/service/auth"
)

func TestSSOCallbackExchangesCodeAndCreatesHttpOnlySessionCookies(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cas/oauth2.0/token":
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token form: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if r.Form.Get("grant_type") != "authorization_code" ||
				r.Form.Get("code") != "one-time-code" ||
				r.Form.Get("redirect_uri") != "https://platform.example/auth/sso/callback" {
				t.Errorf("unexpected token request: %#v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "school-access", "refresh_token": "school-refresh",
				"token_type": "bearer", "expires_in": 28800,
			})
		case "/cas/oauth2.0/profile":
			if r.URL.Query().Get("access_token") != "school-access" {
				t.Errorf("profile request did not include access token")
			}
			_, _ = w.Write([]byte("{\"id\":\"user-1\",\"user\":{\"useraccount\":\"staff-01\",\"username\":\"测试用户\"}}"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	cfg := config.Defaults()
	cfg.AuthMode = "sso"
	cfg.SSOBaseURL = provider.URL + "/cas"
	cfg.SSOClientID = "client"
	cfg.SSOClientSecret = "secret"
	cfg.SSORedirectURL = "https://platform.example/auth/sso/callback"
	cfg.SSOAdminAccounts = "admin-01"
	cfg.JWTSecret = strings.Repeat("secret", 8)
	cfg.AccessTokenTTL = 30 * time.Minute
	cfg.RefreshTokenTTL = time.Hour
	auth := authservice.NewService(nil, cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	app := &Handler{Config: &cfg, Auth: auth}

	request := httptest.NewRequest(http.MethodGet, "https://platform.example/auth/sso/callback?code=one-time-code&state=state-123", nil)
	request.AddCookie(&http.Cookie{Name: ssoStateCookie, Value: "state-123"})
	request.AddCookie(&http.Cookie{Name: ssoReturnCookie, Value: "/IntelligentAI"})
	response := httptest.NewRecorder()
	app.SSOCallback(response, request)

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/IntelligentAI" {
		t.Fatalf("unexpected callback redirect: %d %s", response.Code, response.Header().Get("Location"))
	}
	cookies := response.Result().Cookies()
	values := make(map[string]*http.Cookie, len(cookies))
	for _, cookie := range cookies {
		values[cookie.Name] = cookie
	}
	for _, name := range []string{"Authorization", "RefreshToken", "SSORefreshToken", signedInCookie} {
		if values[name] == nil {
			t.Fatalf("callback did not set %s cookie", name)
		}
	}
	if !values["Authorization"].HttpOnly || !values["RefreshToken"].HttpOnly || !values["SSORefreshToken"].HttpOnly {
		t.Fatal("session and school refresh cookies must be HttpOnly")
	}
	if !values["Authorization"].Secure || values["Authorization"].SameSite != http.SameSiteLaxMode {
		t.Fatal("secure callback cookies must use Secure and SameSite=Lax")
	}
	claims, err := auth.Parse(values["Authorization"].Value)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "staff-01" || claims.Name != "测试用户" || claims.Type != "access" {
		t.Fatalf("unexpected application token claims: %#v", claims)
	}
	if values["SSORefreshToken"].Value != "school-refresh" {
		t.Fatal("school refresh token was not retained for later renewal")
	}
}
