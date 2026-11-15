package setup

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/repository"
	authservice "pingtai/internal/service/auth"
	"pingtai/internal/service/openai"
)

//go:embed setup.html
var page string

var secretFields = []string{
	"JWTSecret", "SSOClientSecret", "ModelAPIKey", "OpenAIAPIKey", "CozeAccessToken", "CozePrivateKey", "CTYunAPIKey", "CTYunAppKey",
}

type Handler struct {
	mutex   sync.Mutex
	current config.Config
	auth    *authservice.Service
	token   string
	pending bool
	prepare func(context.Context, config.Config, string, string) error
}

func New(current config.Config, auth *authservice.Service) *Handler {
	return &Handler{current: current, auth: auth, token: rand.Text(), prepare: prepareDatabase}
}

func (h *Handler) BootstrapToken() string {
	if h.auth != nil {
		return ""
	}
	return h.token
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	if r.URL.Path == "/setup" || r.URL.Path == "/setup/" {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, page)
		return
	}
	if r.URL.Path == "/setup/status" && r.Method == http.MethodGet {
		h.respond(w, http.StatusOK, map[string]any{"bootstrap": h.auth == nil, "sso_enabled": h.current.SSOConfigured()})
		return
	}
	if r.URL.Path != "/setup/api" && r.URL.Path != "/setup/models" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/setup/models" && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	// The custom header also prevents cross-origin simple requests carrying Basic credentials.
	if r.Header.Get("X-Setup-Request") != "1" || !sameOrigin(r) {
		h.respond(w, http.StatusForbidden, map[string]string{"error": "Cross-origin configuration requests are not allowed."})
		return
	}
	if !h.authorized(r) {
		h.respond(w, http.StatusUnauthorized, map[string]string{"error": "Invalid setup key or administrator credentials."})
		return
	}
	h.mutex.Lock()
	defer h.mutex.Unlock()
	if h.pending {
		h.respond(w, http.StatusConflict, map[string]string{"error": "Configuration is saved. Restart the service before making more changes."})
		return
	}
	if r.URL.Path == "/setup/models" {
		h.fetchModels(w, r)
		return
	}
	if r.Method == http.MethodGet {
		h.read(w)
		return
	}
	h.save(w, r)
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && strings.EqualFold(parsed.Host, r.Host)
}

func (h *Handler) authorized(r *http.Request) bool {
	if h.auth == nil {
		return subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Setup-Token")), []byte(h.token)) == 1
	}
	if h.current.SSOConfigured() {
		token := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(token), "bearer ") {
			token = strings.TrimSpace(token[len("Bearer "):])
		}
		if token == "" {
			if cookie, err := r.Cookie("Authorization"); err == nil {
				token = cookie.Value
			}
		}
		claims, err := h.auth.Parse(token)
		return err == nil && claims.Type == "access" && h.current.IsSSOAdmin(claims.Subject)
	}
	username, password, ok := r.BasicAuth()
	if !ok {
		return false
	}
	user, err := h.auth.Authenticate(r.Context(), username, password)
	return err == nil && user.UserType == 1
}


func (h *Handler) read(w http.ResponseWriter) {
	contents, _ := json.Marshal(h.current)
	var visible map[string]any
	_ = json.Unmarshal(contents, &visible)
	configured := make(map[string]bool)
	for _, key := range secretFields {
		configured[key] = visible[key] != ""
		visible[key] = ""
	}
	h.respond(w, http.StatusOK, map[string]any{
		"settings": visible, "configured": configured, "path": config.SettingsPath(), "database_path": h.current.DatabasePath,
	})
}

type saveRequest struct {
	Settings      map[string]json.RawMessage
	ClearSecrets  []string
	AdminUsername string
	AdminPassword string
}

func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 128*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request saveRequest
	if err := decoder.Decode(&request); err != nil {
		h.respond(w, http.StatusBadRequest, map[string]string{"error": "Invalid configuration request (maximum 128 KB)."})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		h.respond(w, http.StatusBadRequest, map[string]string{"error": "Only one JSON object is allowed."})
		return
	}
	candidate, err := h.buildCandidate(request)
	if err != nil {
		h.respond(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	if err := h.prepare(ctx, candidate, request.AdminUsername, request.AdminPassword); err != nil {
		h.respond(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := config.Save(candidate); err != nil {
		h.respond(w, http.StatusInternalServerError, map[string]string{"error": "Cannot save configuration. Check write permission for the executable directory (or PINGTAI_CONFIG_FILE)."})
		return
	}
	h.pending = true
	modelConfigured := candidate.CTYunConfigured()
	if candidate.ChatProvider == "openai" {
		modelConfigured = openai.NewClient(candidate).Configured() && candidate.DefaultModel() != ""
	}
	if candidate.ChatProvider == "coze" {
		modelConfigured = candidate.CozeConfigured()
	}
	h.respond(w, http.StatusOK, map[string]any{
		"saved": true, "restart_required": true, "address": candidate.Addr, "model_configured": modelConfigured,
	})
}

func (h *Handler) buildCandidate(request saveRequest) (config.Config, error) {
	contents, _ := json.Marshal(h.current)
	var merged map[string]json.RawMessage
	_ = json.Unmarshal(contents, &merged)
	for key, value := range request.Settings {
		if _, exists := merged[key]; !exists || key == "AccessTokenTTL" || key == "RefreshTokenTTL" || key == "ModelContextTokens" {
			return config.Config{}, errors.New("Unknown or read-only configuration field.")
		}
		if string(value) == "null" {
			return config.Config{}, errors.New("Configuration fields cannot be null.")
		}
		keepSecret := false
		for _, secret := range secretFields {
			if key == secret && string(value) == `""` {
				keepSecret = true
			}
		}
		if !keepSecret {
			merged[key] = value
		}
	}
	for _, key := range request.ClearSecrets {
		allowed := false
		for _, secret := range secretFields {
			if key == secret {
				allowed = true
			}
		}
		if !allowed {
			return config.Config{}, errors.New("Unknown secret field.")
		}
		merged[key] = json.RawMessage(`""`)
	}
	contents, _ = json.Marshal(merged)
	var candidate config.Config
	if err := json.Unmarshal(contents, &candidate); err != nil {
		return candidate, errors.New("Configuration field has an invalid type.")
	}
	if candidate.JWTSecret == "" || strings.HasPrefix(candidate.JWTSecret, "REPLACE_") {
		candidate.JWTSecret = rand.Text() + rand.Text()
	}
	if err := candidate.Validate(); err != nil {
		return candidate, err
	}
	_, portText, err := net.SplitHostPort(candidate.Addr)
	listenPort, portError := strconv.Atoi(portText)
	if err != nil || portError != nil || listenPort < 1 || listenPort > 65535 {
		return candidate, errors.New("HTTP address must use host:port with port 1-65535, for example :5050.")
	}
	for _, endpoint := range []string{candidate.CTYunAPIURL, candidate.CozeBaseURL} {
		if endpoint == "" {
			continue
		}
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return candidate, errors.New("Model service URLs must be valid HTTP or HTTPS addresses.")
		}
	}
	if candidate.ChatProvider == "openai" || candidate.OpenAIBaseURL != "" {
		if _, err := openai.Endpoint(candidate.OpenAIBaseURL, "chat/completions"); err != nil {
			return candidate, err
		}
	}
	return candidate, nil
}

func prepareDatabase(ctx context.Context, candidate config.Config, username, password string) error {
	db, err := database.Open(ctx, candidate.DatabasePath)
	if err != nil {
		return errors.New("Cannot open the SQLite database file. Check the configured path and parent-directory permissions.")
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		return errors.New("Cannot initialize SQLite tables. Check the database file and directory permissions.")
	}
	if candidate.SSOConfigured() {
		return nil
	}
	var admins int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_user WHERE usertype=1 AND status=1 AND COALESCE(deltag,0)=0").Scan(&admins); err != nil {
		return errors.New("Cannot read administrator records.")
	}
	if admins > 0 {
		return nil // Never reset an existing administrator during setup or recovery.
	}
	username = strings.TrimSpace(username)
	if username == "" || len([]rune(username)) > 50 || len(password) < 8 || len(password) > 72 {
		return errors.New("This database needs an initial administrator: username 1-50 characters, password 8-72 bytes.")
	}
	hashed, err := authservice.HashPassword(password)
	if err != nil {
		return errors.New("Cannot hash administrator password.")
	}
	users := repository.NewUserRepository(db)
	if _, err := users.Create(ctx, username, hashed, 1); err != nil {
		return errors.New("Cannot create administrator. The username may already exist; choose a different username.")
	}
	return nil
}

func (h *Handler) respond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
