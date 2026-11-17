package config

import (
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr               string
	AuthMode           string
	LocalLoginEnabled  bool
	SSOLoginEnabled    bool
	DatabasePath       string
	SSOBaseURL         string
	SSOServiceURL      string
	SSOClientID        string
	SSOClientSecret    string
	SSORedirectURL     string
	SSOAdminAccounts   string
	JWTSecret          string
	WebDir             string
	ChatProvider       string
	AccessTokenTTL     time.Duration
	RefreshTokenTTL    time.Duration
	MaxUploadBytes     int64
	ModelAPIKey        string
	OpenAIBaseURL      string
	OpenAIAPIKey       string
	OpenAIModel        string
	OpenAIModels       []string
	OpenAISystemPrompt string
	OpenAITimeout      time.Duration
	OpenAIMaxTokens    int
	ModelContextTokens int64
	ContextTurns       int
	CozeBaseURL        string
	CozeAccessToken    string
	CozeAppID          string
	CozePublicKeyID    string
	CozePrivateKey     string
	CozePrivateKeyFile string
	CozeWorkspaceID    string
	CTYunAPIURL        string
	CTYunModel         string
	CTYunAuthMode      string
	CTYunAPIKey        string
	CTYunAppID         string
	CTYunAppKey        string
	CTYunRegion        string
	CTYunSignPrefix    string
	CTYunOrderNum      string
	CTYunKnowledgeBase string
	CTYunSystemPrompt  string
	CTYunTimeout       time.Duration
	CTYunMaxTokens     int
}

func Load() (Config, error) {
	// Load .env before reading saved settings so PINGTAI_DB_PATH and
	// PINGTAI_CONFIG_FILE are available during upgrade-path resolution.
	loadConfigDotEnv()
	if saved, exists, err := loadSaved(); exists || err != nil {
		return saved, err
	}
	if strings.TrimSpace(os.Getenv("MYSQL_DSN")) != "" || strings.TrimSpace(os.Getenv("DATABASE_URL")) != "" {
		return Config{}, errors.New("legacy MySQL connection settings detected; export and migrate existing data to SQLite before removing MYSQL_DSN or DATABASE_URL")
	}


	workspaceID := env("COZE_WORKSPACE_ID", "7481824441182617636")
	loginMode := strings.ToLower(env("AUTH_MODE", "local"))
	localLoginEnabled := envBool("LOCAL_LOGIN_ENABLED", true)
	ssoLoginEnabled := envBool("SSO_LOGIN_ENABLED", loginMode == "sso" || loginMode == "multi")
	if ssoLoginEnabled {
		if localLoginEnabled {
			loginMode = "multi"
		} else {
			loginMode = "sso"
		}
	} else {
		loginMode = "local"
	}
	cozePrivateKeyFile := strings.TrimSpace(os.Getenv("COZE_PRIVATE_KEY_FILE"))
	if cozePrivateKeyFile == "" {
		if _, statErr := os.Stat("backend/app/private_key.pem"); statErr == nil {
			cozePrivateKeyFile = "backend/app/private_key.pem"
		}
	}
	cozeAppID := env("COZE_APP_ID", strings.TrimSpace(os.Getenv("COZE_CLIENT_ID")))
	authMode := strings.ToLower(env("CTYUN_AUTH_MODE", "bearer"))
	if authMode != "bearer" && authMode != "signature" {
		return Config{}, errors.New("CTYUN_AUTH_MODE must be bearer or signature")
	}
	selectedModels := strings.FieldsFunc(os.Getenv("OPENAI_MODELS"), func(character rune) bool { return character == ',' || character == '\n' })
	for index := range selectedModels {
		selectedModels[index] = strings.TrimSpace(selectedModels[index])
	}

	return Config{
		Addr:               env("HTTP_ADDR", ":5050"),
		AuthMode:           loginMode,
		LocalLoginEnabled:  localLoginEnabled,
		SSOLoginEnabled:    ssoLoginEnabled,
		DatabasePath:       env("PINGTAI_DB_PATH", DefaultDatabasePath()),
		SSOBaseURL:         strings.TrimRight(env("SSO_BASE_URL", "https://ids.zcvc.cn/cas"), "/"),
		SSOServiceURL:      strings.TrimSpace(os.Getenv("SSO_SERVICE_URL")),
		SSOClientID:        strings.TrimSpace(os.Getenv("SSO_CLIENT_ID")),
		SSOClientSecret:    strings.TrimSpace(os.Getenv("SSO_CLIENT_SECRET")),
		SSORedirectURL:     strings.TrimSpace(os.Getenv("SSO_REDIRECT_URL")),
		SSOAdminAccounts:   strings.TrimSpace(os.Getenv("SSO_ADMIN_ACCOUNTS")),
		JWTSecret:          strings.TrimSpace(os.Getenv("JWT_SECRET_KEY")),
		WebDir:             strings.TrimSpace(os.Getenv("WEB_DIR")),
		ChatProvider:       strings.ToLower(env("CHAT_PROVIDER", "openai")),
		AccessTokenTTL:     30 * time.Minute,
		RefreshTokenTTL:    30 * 24 * time.Hour,
		MaxUploadBytes:     int64(envInt("MAX_UPLOAD_MB", 100)) * 1024 * 1024,
		ModelAPIKey:        strings.TrimSpace(os.Getenv("MODEL_API_KEY")),
		OpenAIBaseURL:      env("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIAPIKey:       strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIModel:        strings.TrimSpace(os.Getenv("OPENAI_MODEL")),
		OpenAIModels:       selectedModels,
		OpenAISystemPrompt: strings.TrimSpace(os.Getenv("OPENAI_SYSTEM_PROMPT")),
		OpenAITimeout:      time.Duration(envInt("OPENAI_TIMEOUT_SECONDS", 180)) * time.Second,
		OpenAIMaxTokens:    envInt("OPENAI_MAX_TOKENS", 0),
		ContextTurns:       envInt("CONTEXT_TURNS", 20),
		CozeBaseURL:        strings.TrimRight(env("COZE_BASE_URL", "https://api.coze.cn"), "/"),
		CozeAccessToken:    strings.TrimSpace(os.Getenv("COZE_ACCESS_TOKEN")),
		CozeAppID:          cozeAppID,
		CozePublicKeyID:    env("COZE_PUBLIC_KEY_ID", strings.TrimSpace(os.Getenv("COZE_PUBLIC_KEY"))),
		CozePrivateKey:     strings.TrimSpace(os.Getenv("COZE_PRIVATE_KEY")),
		CozePrivateKeyFile: cozePrivateKeyFile,
		CozeWorkspaceID:    workspaceID,
		CTYunAPIURL:        strings.TrimSpace(os.Getenv("CTYUN_API_URL")),
		CTYunModel:         strings.TrimSpace(os.Getenv("CTYUN_MODEL")),
		CTYunAuthMode:      authMode,
		CTYunAPIKey:        strings.TrimSpace(os.Getenv("CTYUN_API_KEY")),
		CTYunAppID:         strings.TrimSpace(os.Getenv("CTYUN_APP_ID")),
		CTYunAppKey:        strings.TrimSpace(os.Getenv("CTYUN_APP_KEY")),
		CTYunRegion:        env("CTYUN_REGION", "QG"),
		CTYunSignPrefix:    env("CTYUN_SIGN_PREFIX", "teleai-cloud-auth-v1"),
		CTYunOrderNum:      strings.TrimSpace(os.Getenv("CTYUN_ORDER_NUM")),
		CTYunKnowledgeBase: strings.TrimSpace(os.Getenv("CTYUN_KNOWLEDGE_BASE")),
		CTYunSystemPrompt:  strings.TrimSpace(os.Getenv("CTYUN_SYSTEM_PROMPT")),
		CTYunTimeout:       time.Duration(envInt("CTYUN_TIMEOUT_SECONDS", 180)) * time.Second,
		CTYunMaxTokens:     envInt("CTYUN_MAX_COMPLETION_TOKENS", 1024),
	}, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.DatabasePath) == "" {
		return errors.New("PINGTAI_DB_PATH is required")
	}
	authMode := strings.ToLower(strings.TrimSpace(c.AuthMode))
	if authMode == "" {
		authMode = "local"
	}
	if authMode != "local" && authMode != "sso" && authMode != "multi" {
		return errors.New("AUTH_MODE must be local, sso or multi")
	}
	ssoLoginEnabled := c.SSOLoginEnabled || authMode == "sso" || authMode == "multi"
	if !c.LocalLoginEnabled && !ssoLoginEnabled {
		return errors.New("at least one login method must be enabled")
	}
	if c.SSOSettingsComplete() {
		base, err := url.Parse(c.SSOBaseURL)
		if err != nil || base.Hostname() == "" || (base.Scheme != "https" && base.Scheme != "http") {
			return errors.New("SSO_BASE_URL must be a valid HTTP or HTTPS URL")
		}
		redirect, err := url.Parse(c.SSORedirectURL)
		if err != nil || redirect.Hostname() == "" || (redirect.Scheme != "https" && redirect.Scheme != "http") {
			return errors.New("SSO_REDIRECT_URL must be a valid HTTP or HTTPS URL")
		}
		if service := strings.TrimSpace(c.SSOServiceURL); service != "" {
			parsed, serviceErr := url.Parse(service)
			if serviceErr != nil || parsed.Hostname() == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
				return errors.New("SSO_SERVICE_URL must be a valid HTTP or HTTPS URL")
			}
		}
	}
	if ssoLoginEnabled {
		if !c.SSOSettingsComplete() {
			return errors.New("SSO mode requires SSO_CLIENT_ID, SSO_CLIENT_SECRET and SSO_REDIRECT_URL")
		}
		if strings.TrimSpace(c.SSOAdminAccounts) == "" {
			return errors.New("SSO_ADMIN_ACCOUNTS must contain at least one administrator account")
		}
	}
	if c.JWTSecret == "" {
		return errors.New("JWT_SECRET_KEY is required")
	}
	if len(c.JWTSecret) < 32 {
		return errors.New("JWT_SECRET_KEY must contain at least 32 characters")
	}
	if c.ChatProvider != "openai" && c.ChatProvider != "ctyun" && c.ChatProvider != "coze" {
		return errors.New("CHAT_PROVIDER must be openai, ctyun or coze")
	}
	if c.MaxUploadBytes <= 0 {
		return errors.New("MAX_UPLOAD_MB must be greater than zero")
	}
	if c.CTYunAuthMode != "bearer" && c.CTYunAuthMode != "signature" {
		return errors.New("CTYUN_AUTH_MODE must be bearer or signature")
	}
	if c.CTYunTimeout <= 0 || c.CTYunMaxTokens <= 0 {
		return errors.New("model timeout and maximum tokens must be greater than zero")
	}
	if c.ChatProvider == "openai" && (c.OpenAITimeout <= 0 || c.OpenAIMaxTokens < 0) {
		return errors.New("OpenAI timeout must be positive and maximum tokens must be zero or positive")
	}
	if c.ModelContextTokens < 0 {
		return errors.New("model context tokens cannot be negative")
	}
	if c.ContextTurns < 0 || c.ContextTurns > 1000 {
		return errors.New("context turns must be between 0 (all history) and 1000")
	}
	seenModels := make(map[string]bool)
	for _, model := range c.OpenAIModels {
		if strings.TrimSpace(model) == "" || model != strings.TrimSpace(model) || seenModels[model] {
			return errors.New("selected models must be non-empty, trimmed and unique")
		}
		seenModels[model] = true
	}
	if len(c.OpenAIModels) > 0 && c.OpenAIModel != "" && !seenModels[c.OpenAIModel] {
		return errors.New("default model must be included in the selected models")
	}
	return nil
}

// WebModels keeps old single-model configurations working without a migration.
func (c Config) WebModels() []string {
	if len(c.OpenAIModels) > 0 {
		models := append([]string(nil), c.OpenAIModels...)
		for index, model := range models {
			if model == c.OpenAIModel {
				copy(models[1:index+1], models[:index])
				models[0] = model
				break
			}
		}
		return models
	}
	if c.OpenAIModel != "" {
		return []string{c.OpenAIModel}
	}
	return nil
}


func (c Config) SSOSettingsComplete() bool {
	return strings.TrimSpace(c.SSOClientID) != "" &&
		strings.TrimSpace(c.SSOClientSecret) != "" &&
		strings.TrimSpace(c.SSORedirectURL) != ""
}

func (c Config) SSOConfigured() bool {
	mode := strings.ToLower(strings.TrimSpace(c.AuthMode))
	return (c.SSOLoginEnabled || mode == "sso" || mode == "multi") && c.SSOSettingsComplete()
}

func (c Config) LoginMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.AuthMode))
	ssoEnabled := c.SSOLoginEnabled || mode == "sso" || mode == "multi"
	switch {
	case c.LocalLoginEnabled && ssoEnabled:
		return "multi"
	case ssoEnabled:
		return "sso"
	default:
		return "local"
	}
}

func (c Config) IsSSOAdmin(account string) bool {
	account = strings.TrimSpace(account)
	if account == "" {
		return false
	}
	for _, allowed := range strings.FieldsFunc(c.SSOAdminAccounts, func(character rune) bool {
		return character == ',' || character == ';' || character == '\n' || character == '\r'
	}) {
		if strings.EqualFold(strings.TrimSpace(allowed), account) {
			return true
		}
	}
	return false
}

func (c Config) UsesLocalConversations() bool {
	return c.ChatProvider == "openai" || c.ChatProvider == "ctyun"
}

func (c Config) DefaultModel() string {
	if c.ChatProvider == "openai" {
		if c.OpenAIModel == "" && len(c.OpenAIModels) > 0 {
			return c.OpenAIModels[0]
		}
		return c.OpenAIModel
	}
	return c.CTYunModel
}

func (c Config) SystemPrompt() string {
	if c.ChatProvider == "openai" {
		return c.OpenAISystemPrompt
	}
	return c.CTYunSystemPrompt
}

func (c Config) CozeConfigured() bool {
	if c.CozeAccessToken != "" {
		return true
	}
	hasPrivateKey := c.CozePrivateKey != ""
	if !hasPrivateKey && c.CozePrivateKeyFile != "" {
		_, err := os.Stat(c.CozePrivateKeyFile)
		hasPrivateKey = err == nil
	}
	return c.CozeAppID != "" && c.CozePublicKeyID != "" && hasPrivateKey
}

func (c Config) CTYunConfigured() bool {
	if c.CTYunAPIURL == "" {
		return false
	}
	if c.CTYunAuthMode == "signature" {
		return c.CTYunAppID != "" && c.CTYunAppKey != "" && c.CTYunRegion != ""
	}
	return c.CTYunAPIKey != ""
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

