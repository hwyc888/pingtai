package sso

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pingtai/internal/config"
)

func TestRefreshTokenUsesOAuth2RefreshGrant(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/cas/oauth2.0/token" {
			t.Fatalf("unexpected refresh request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" ||
			r.Form.Get("client_id") != "client" ||
			r.Form.Get("client_secret") != "secret" ||
			r.Form.Get("refresh_token") != "school-refresh" {
			t.Fatalf("unexpected refresh form: %#v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{AccessToken: "renewed-access"})
	}))
	defer server.Close()

	cfg := config.Defaults()
	cfg.SSOBaseURL = server.URL + "/cas"
	cfg.SSOClientID = "client"
	cfg.SSOClientSecret = "secret"
	client := NewClient(cfg)
	client.httpClient = server.Client()

	token, err := client.RefreshToken(context.Background(), "school-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "renewed-access" {
		t.Fatalf("unexpected refreshed token: %#v", token)
	}
}
