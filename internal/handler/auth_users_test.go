package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"strings"

	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/repository"
	authservice "pingtai/internal/service/auth"
)

func TestAddUserAllowsNormalUserRoleZero(t *testing.T) {
	db, err := database.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	users := repository.NewUserRepository(db)
	cfg := config.Defaults()
	cfg.JWTSecret = "test-jwt-secret-with-more-than-32-bytes"
	appAuth := authservice.NewService(users, cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	app := &Handler{Config: &cfg, DB: db, Users: users, Auth: appAuth}

	body, _ := json.Marshal(map[string]any{
		"username": "student01",
		"password": "student-password-123",
		"usertype": 0,
	})
	request := httptest.NewRequest(http.MethodPost, "/auth/addUser", strings.NewReader(string(body)))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.AddUser(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("add user returned HTTP %d: %s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if intValue(result["errcode"], 0) != 200 {
		t.Fatalf("normal user creation failed: %s", response.Body.String())
	}
	user, err := users.FindByUsername(context.Background(), "student01")
	if err != nil || user.UserType != 0 || user.Status != 1 {
		t.Fatalf("normal user was not stored correctly: %#v err=%v", user, err)
	}
}
