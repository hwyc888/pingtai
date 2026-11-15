package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/handler"
	"pingtai/internal/httpapi"
	"pingtai/internal/model"
	"pingtai/internal/repository"
	authservice "pingtai/internal/service/auth"
	"pingtai/internal/service/coze"
	"pingtai/internal/service/openai"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load platform configuration: %w", err)
	}
	if len(os.Args) > 1 && os.Args[1] == "create-admin" {
		return runCreateAdmin(cfg, os.Args[2:])
	}

	// Platform-local accounts and OpenAI-compatible providers are managed in the UI.
	cfg.AuthMode = "local"
	cfg.SSOBaseURL = ""
	cfg.SSOClientID = ""
	cfg.SSOClientSecret = ""
	cfg.SSORedirectURL = ""
	cfg.SSOAdminAccounts = ""
	cfg.ChatProvider = "openai"

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, cleanup, err := buildApplication(ctx, cfg)
	if err != nil {
		return err
	}
	defer cleanup()

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           app,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}
	address := browserAddress(listener.Addr().String())
	log.Printf("PINGTAI platform: %s", address)
	log.Printf("First visit: create the administrator account in the browser.")
	log.Printf("SQLite database: %s", cfg.DatabasePath)

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Go platform listening on %s", cfg.Addr)
		serverErrors <- server.Serve(listener)
	}()
	if runtime.GOOS == "windows" && len(os.Args) > 1 && os.Args[1] == "--open-browser" {
		command := exec.Command("rundll32", "url.dll,FileProtocolHandler", address)
		if err := command.Start(); err != nil {
			log.Printf("Could not open the browser; open the URL above manually.")
		} else {
			go func() { _ = command.Wait() }()
		}
	}

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func buildApplication(ctx context.Context, cfg config.Config) (http.Handler, func(), error) {
	db, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot open SQLite database at %q: %w", cfg.DatabasePath, err)
	}
	cleanup := func() { _ = db.Close() }
	if err := database.Migrate(ctx, db); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("SQLite database migration failed: %w", err)
	}
	if err := ensureJWTSecret(ctx, db, &cfg); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("cannot initialize platform authentication secret: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("invalid platform configuration: %w", err)
	}

	providers := repository.NewProviderRepository(db)
	if err := importLegacyProvider(ctx, cfg, providers); err != nil {
		cleanup()
		return nil, nil, errors.New("cannot migrate legacy AI provider configuration")
	}
	cfg.ModelContextTokens = 0
	if active, models, providerErr := providers.Active(ctx); providerErr == nil {
		cfg.ChatProvider = "openai"
		cfg.OpenAIBaseURL = active.BaseURL
		cfg.OpenAIAPIKey = active.APIKey
		cfg.OpenAIModel = active.DefaultModel
		cfg.OpenAIMaxTokens = active.MaxOutputTokens
		cfg.OpenAITimeout = time.Duration(active.TimeoutSeconds) * time.Second
		cfg.OpenAISystemPrompt = active.SystemPrompt
		cfg.OpenAIModels = make([]string, 0, len(models))
		for _, item := range models {
			cfg.OpenAIModels = append(cfg.OpenAIModels, item.ModelID)
			if item.ModelID == active.DefaultModel {
				if item.ContextLength != nil {
					cfg.ModelContextTokens = *item.ContextLength
				}
				if cfg.OpenAIMaxTokens == 0 && item.MaxOutputTokens != nil {
					cfg.OpenAIMaxTokens = int(*item.MaxOutputTokens)
				}
			}
		}
	} else if !errors.Is(providerErr, sql.ErrNoRows) {
		cleanup()
		return nil, nil, errors.New("cannot load active AI provider from SQLite")
	}

	users := repository.NewUserRepository(db)
	chats := repository.NewChatRepository(db)
	auth := authservice.NewService(users, cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	modelClient := openai.NewClient(cfg)
	appHandler := handler.New(&cfg, db, users, chats, auth, coze.NewClient(cfg), modelClient)
	return httpapi.NewRouter(&cfg, appHandler), cleanup, nil
}

func ensureJWTSecret(ctx context.Context, db *sql.DB, cfg *config.Config) error {
	var stored string
	err := db.QueryRowContext(ctx, "SELECT setting_value FROM ai_platform_setting WHERE setting_key='jwt_secret'").Scan(&stored)
	if err == nil && strings.TrimSpace(stored) != "" {
		cfg.JWTSecret = stored
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	secret := strings.TrimSpace(cfg.JWTSecret)
	if len(secret) < 32 {
		randomBytes := make([]byte, 48)
		if _, err := rand.Read(randomBytes); err != nil {
			return err
		}
		secret = base64.RawURLEncoding.EncodeToString(randomBytes)
	}
	_, err = db.ExecContext(ctx, `
		INSERT OR IGNORE INTO ai_platform_setting (setting_key, setting_value, updated_at)
		VALUES ('jwt_secret', ?, CURRENT_TIMESTAMP)
	`, secret)
	if err != nil {
		return err
	}
	return db.QueryRowContext(ctx, "SELECT setting_value FROM ai_platform_setting WHERE setting_key='jwt_secret'").Scan(&cfg.JWTSecret)
}

func browserAddress(address string) string {
	host, port, _ := net.SplitHostPort(address)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
