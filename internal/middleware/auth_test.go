package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	authservice "pingtai/internal/service/auth"
)

func TestAccessAndRefreshTokensCanBeReadFromHttpOnlyCookies(t *testing.T) {
	tokens := authservice.NewTokenManager(strings.Repeat("secret", 8), time.Minute, time.Hour)
	pair, err := tokens.IssuePairWithName("staff-01", "Test User")
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := IdentityFromContext(r.Context())
		if !ok || identity.Username != "staff-01" || identity.DisplayName != "Test User" {
			t.Fatalf("unexpected identity: %#v, ok=%v", identity, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	access := httptest.NewRecorder()
	accessRequest := httptest.NewRequest(http.MethodGet, "/protected", nil)
	accessRequest.AddCookie(&http.Cookie{Name: "Authorization", Value: pair.AccessToken})
	RequireAccess(tokens, next).ServeHTTP(access, accessRequest)
	if access.Code != http.StatusNoContent {
		t.Fatalf("access cookie rejected: %d", access.Code)
	}

	refresh := httptest.NewRecorder()
	refreshRequest := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	refreshRequest.AddCookie(&http.Cookie{Name: "RefreshToken", Value: pair.RefreshToken})
	RequireRefresh(tokens, next).ServeHTTP(refresh, refreshRequest)
	if refresh.Code != http.StatusNoContent {
		t.Fatalf("refresh cookie rejected: %d", refresh.Code)
	}
}

func TestRequireAdminAccountsUsesSSOAccountAllowlist(t *testing.T) {
	tokens := authservice.NewTokenManager(strings.Repeat("secret", 8), time.Minute, time.Hour)
	pair, err := tokens.IssuePair("admin-01")
	if err != nil {
		t.Fatal(err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	protected := RequireAdminAccounts(tokens, "admin-01;admin-02", next)

	allowed := httptest.NewRecorder()
	allowedRequest := httptest.NewRequest(http.MethodGet, "/admin", nil)
	allowedRequest.AddCookie(&http.Cookie{Name: "Authorization", Value: pair.AccessToken})
	protected.ServeHTTP(allowed, allowedRequest)
	if allowed.Code != http.StatusNoContent {
		t.Fatalf("configured administrator was rejected: %d", allowed.Code)
	}

	ordinaryPair, err := tokens.IssuePair("student-01")
	if err != nil {
		t.Fatal(err)
	}
	denied := httptest.NewRecorder()
	deniedRequest := httptest.NewRequest(http.MethodGet, "/admin", nil)
	deniedRequest.AddCookie(&http.Cookie{Name: "Authorization", Value: ordinaryPair.AccessToken})
	protected.ServeHTTP(denied, deniedRequest)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("ordinary user was not denied: %d", denied.Code)
	}
}
