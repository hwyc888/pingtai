package sso

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"pingtai/internal/config"
)

func TestOAuthAuthorizationCodeAndProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cas/oauth2.0/token":
			if r.Method != http.MethodPost {
				t.Fatalf("token method = %s", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("client_id") != "client" ||
				r.Form.Get("client_secret") != "secret" ||
				r.Form.Get("grant_type") != "authorization_code" ||
				r.Form.Get("code") != "single-use-code" ||
				r.Form.Get("redirect_uri") != "https://app.example/auth/sso/callback" {
				t.Fatalf("unexpected token form: %#v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(TokenResponse{
				AccessToken: "access", RefreshToken: "refresh", TokenType: "bearer", ExpiresIn: 28800,
			})
		case "/cas/oauth2.0/profile":
			if r.Method != http.MethodGet || r.URL.Query().Get("access_token") != "access" {
				t.Fatalf("unexpected profile request: %s %s", r.Method, r.URL.String())
			}
			_, _ = w.Write([]byte("{\"id\":\"user-1\",\"user\":{\"account\":\"staff\",\"useraccount\":\"staff-01\",\"username\":\"测试用户\",\"phone\":\"123\",\"email\":\"test@example.com\"}}"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.SSOBaseURL = server.URL + "/cas"
	cfg.SSOClientID = "client"
	cfg.SSOClientSecret = "secret"
	cfg.SSORedirectURL = "https://app.example/auth/sso/callback"
	client := NewClient(cfg)
	client.httpClient = server.Client()

	authorizationURL, err := client.AuthorizationURL("csrf-state")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/cas/oauth2.0/authorize" ||
		parsed.Query().Get("response_type") != "code" ||
		parsed.Query().Get("client_id") != "client" ||
		parsed.Query().Get("redirect_uri") != cfg.SSORedirectURL ||
		parsed.Query().Get("state") != "csrf-state" {
		t.Fatalf("unexpected authorization URL: %s", authorizationURL)
	}

	token, err := client.ExchangeCode(context.Background(), "single-use-code")
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "access" || token.RefreshToken != "refresh" || token.ExpiresIn != 28800 {
		t.Fatalf("unexpected token: %#v", token)
	}
	profile, err := client.UserProfile(context.Background(), token.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Account != "staff-01" || profile.DisplayName != "测试用户" || profile.ID != "user-1" {
		t.Fatalf("unexpected profile: %#v", profile)
	}
}

func TestParseUserProfileFallbackAndMissingAccount(t *testing.T) {
	profile, err := ParseUserProfile([]byte("{\"id\":\"admin\",\"username\":\"系统管理员\",\"user\":null}"))
	if err != nil {
		t.Fatal(err)
	}
	if profile.Account != "admin" || profile.DisplayName != "系统管理员" {
		t.Fatalf("unexpected fallback profile: %#v", profile)
	}
	if _, err := ParseUserProfile([]byte("{\"user\":{\"username\":\"没有账号字段\"}}")); err == nil {
		t.Fatal("profile without a stable account identifier must be rejected")
	}
}

func TestParseUserProfileSupportsSchoolUserInfoShape(t *testing.T) {
	profile, err := ParseUserProfile([]byte("{\"service\":\"https://platform.example/\",\"id\":\"admin\",\"client_id\":\"client\",\"user_info\":{\"phone\":null,\"account\":\"admin\",\"useraccount\":\"admin\",\"email\":null,\"username\":\"系统管理员\"}}"))
	if err != nil {
		t.Fatal(err)
	}
	if profile.Account != "admin" || profile.DisplayName != "系统管理员" || profile.ID != "admin" {
		t.Fatalf("unexpected school profile: %#v", profile)
	}
}
