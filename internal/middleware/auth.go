package middleware

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"pingtai/internal/repository"
	authservice "pingtai/internal/service/auth"
)

type identityKey struct{}

type Identity struct {
	Username    string
	DisplayName string
	JTI         string
	Expires  int64
	APIKey   bool
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	value, ok := ctx.Value(identityKey{}).(Identity)
	return value, ok
}

func RequireAccess(tokens *authservice.TokenManager, next http.Handler) http.Handler {
	return requireToken(tokens, "access", next)
}

func RequireRefresh(tokens *authservice.TokenManager, next http.Handler) http.Handler {
	return requireToken(tokens, "refresh", next)
}

func RequireAdmin(tokens *authservice.TokenManager, users *repository.UserRepository, next http.Handler) http.Handler {
	return RequireAccess(tokens, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			writeAuthError(w, http.StatusUnauthorized, "Invalid token")
			return
		}
		user, err := users.FindByUsername(r.Context(), identity.Username)
		if err != nil || user.UserType != 1 || user.Status != 1 || user.DelTag != 0 {
			writeAuthError(w, http.StatusForbidden, "权限受限，非管理员用户")
			return
		}
		next.ServeHTTP(w, r)
	}))
}


func RequireAdminAccounts(tokens *authservice.TokenManager, accounts string, next http.Handler) http.Handler {
	return RequireAccess(tokens, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := IdentityFromContext(r.Context())
		if !ok || !isAllowedAdmin(accounts, identity.Username) {
			writeAuthError(w, http.StatusForbidden, "权限受限，非管理员用户")
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func isAllowedAdmin(accounts, username string) bool {
	username = strings.TrimSpace(username)
	if username == "" {
		return false
	}
	for _, account := range strings.FieldsFunc(accounts, func(character rune) bool {
		return character == ',' || character == ';' || character == '\n' || character == '\r'
	}) {
		if strings.EqualFold(strings.TrimSpace(account), username) {
			return true
		}
	}
	return false
}

func RequireAPIOrAccess(tokens *authservice.TokenManager, apiKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiKey != "" {
			provided := strings.TrimSpace(r.Header.Get("X-API-Key"))
			if provided == "" {
				provided = bearerToken(r.Header.Get("Authorization"))
			}
			if provided != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(apiKey)) == 1 {
				ctx := context.WithValue(r.Context(), identityKey{}, Identity{Username: "api-client", APIKey: true})
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		claims, err := tokens.Parse(bearerToken(r.Header.Get("Authorization")))
		if err != nil || claims.Type != "access" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
				"message": "Invalid or missing API key", "type": "authentication_error", "param": nil, "code": "invalid_api_key",
			}})
			return
		}
		identity := Identity{Username: claims.Subject, DisplayName: claims.Name, JTI: claims.JTI, Expires: claims.Expires}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, identity)))
	})
}

func requireToken(tokens *authservice.TokenManager, expectedType string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			cookieName := "Authorization"
			if expectedType == "refresh" {
				cookieName = "RefreshToken"
			}
			if cookie, err := r.Cookie(cookieName); err == nil {
				token = cookie.Value
			}
		}
		if token == "" {
			writeAuthError(w, http.StatusUnauthorized, "Missing Authorization Header")
			return
		}
		claims, err := tokens.Parse(token)
		if err != nil || claims.Type != expectedType {
			writeAuthError(w, http.StatusUnauthorized, "Invalid token")
			return
		}
		identity := Identity{Username: claims.Subject, DisplayName: claims.Name, JTI: claims.JTI, Expires: claims.Expires}
		ctx := context.WithValue(r.Context(), identityKey{}, identity)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(value string) string {
	parts := strings.SplitN(strings.TrimSpace(value), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"code": status, "msg": message})
}
