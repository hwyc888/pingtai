package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/repository"
	authservice "pingtai/internal/service/auth"
)

func TestLocalAccountLoginRemainsAvailableWhenSSOIsAlsoEnabled(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	users := repository.NewUserRepository(db)
	passwordHash, err := authservice.HashPassword("local-password-123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.Create(ctx, "local-user", passwordHash, 0); err != nil {
		t.Fatal(err)
	}

	cfg := config.Defaults()
	cfg.JWTSecret = strings.Repeat("s", 40)
	cfg.LocalLoginEnabled = true
	cfg.SSOLoginEnabled = true
	cfg.AuthMode = "multi"
	cfg.SSOClientID = "client"
	cfg.SSOClientSecret = "secret"
	cfg.SSORedirectURL = "https://platform.example/auth/sso/callback"
	cfg.SSOAdminAccounts = "school-admin"
	cfg.AccessTokenTTL = 30 * time.Minute
	cfg.RefreshTokenTTL = 24 * time.Hour

	handler := &Handler{
		Config: &cfg,
		DB: db,
		Users: users,
		Auth: authservice.NewService(users, cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL),
	}
	form := url.Values{"username": {"local-user"}, "password": {"local-password-123"}}
	request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.Login(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"errcode":200`) {
		t.Fatalf("local login should remain available with SSO enabled: status=%d body=%s", response.Code, response.Body.String())
	}
	foundLocalMarker := false
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "pingtai_auth_method" && cookie.Value == "local" {
			foundLocalMarker = true
		}
	}
	if !foundLocalMarker {
		t.Fatal("local login should mark the session as local for the correct logout flow")
	}
}

func TestLocalAccountLoginCanBeDisabledIndependentlyOfSSO(t *testing.T) {
	cfg := config.Defaults()
	cfg.LocalLoginEnabled = false
	cfg.SSOLoginEnabled = true
	cfg.AuthMode = "sso"
	handler := &Handler{Config: &cfg}
	request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("username=alice&password=password"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.Login(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("disabled local login should be rejected while SSO remains enabled: status=%d body=%s", response.Code, response.Body.String())
	}
}
