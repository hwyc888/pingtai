package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pingtai/internal/config"
)

func TestAuthModeReportsConfiguredMode(t *testing.T) {
	for _, test := range []struct {
		name string
		mode string
		want string
	}{
		{name: "default local", mode: "local", want: "local"},
		{name: "sso", mode: "sso", want: "sso"},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.AuthMode = test.mode
			if test.mode == "sso" {
				cfg.SSOClientID = "client"
				cfg.SSOClientSecret = "secret"
				cfg.SSORedirectURL = "https://platform.example/auth/sso/callback"
				cfg.SSOAdminAccounts = "admin"
			}
			app := &Handler{Config: &cfg}
			response := httptest.NewRecorder()
			app.AuthMode(response, httptest.NewRequest(http.MethodGet, "/auth/mode", nil))
			expected := "\"mode\":\"" + test.want + "\""
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), expected) {
				t.Fatalf("unexpected auth mode response: %d %s", response.Code, response.Body.String())
			}
		})
	}
}
