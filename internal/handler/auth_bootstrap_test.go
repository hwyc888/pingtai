package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/repository"
	authservice "pingtai/internal/service/auth"
)

func TestFirstRegistrationCreatesAdminAndClosesBootstrap(t *testing.T) {
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
	cfg := config.Defaults()
	cfg.JWTSecret = strings.Repeat("test-secret-", 4)
	appAuth := authservice.NewService(users, cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	app := &Handler{Config: &cfg, DB: db, Users: users, Auth: appAuth}

	status := httptest.NewRecorder()
	app.BootstrapStatus(status, httptest.NewRequest(http.MethodGet, "/auth/bootstrap/status", nil))
	var statusBody struct {
		Data struct {
			NeedsAdmin bool `json:"needs_admin"`
		} `json:"data"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &statusBody); err != nil || !statusBody.Data.NeedsAdmin {
		t.Fatalf("fresh database should require an administrator: %s err=%v", status.Body.String(), err)
	}

	body := `{"username":"first-admin","password":"strong-password-123"}`
	request := httptest.NewRequest(http.MethodPost, "http://platform.example/auth/bootstrap/register", strings.NewReader(body))
	request.Host = "platform.example"
	request.Header.Set("Origin", "http://platform.example")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.RegisterFirstAdmin(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("first registration failed: HTTP %d %s", response.Code, response.Body.String())
	}
	user, err := users.FindByUsername(ctx, "first-admin")
	if err != nil || user.UserType != 1 || user.Status != 1 {
		t.Fatalf("first account is not an active administrator: %#v err=%v", user, err)
	}
	if len(response.Result().Cookies()) == 0 {
		t.Fatal("successful registration should establish a login session")
	}

	second := httptest.NewRequest(http.MethodPost, "http://platform.example/auth/bootstrap/register", strings.NewReader(body))
	second.Header.Set("Origin", "http://platform.example")
	second.Header.Set("Content-Type", "application/json")
	secondResponse := httptest.NewRecorder()
	app.RegisterFirstAdmin(secondResponse, second)
	if secondResponse.Code != http.StatusConflict {
		t.Fatalf("second registration should be closed, got HTTP %d: %s", secondResponse.Code, secondResponse.Body.String())
	}
}
