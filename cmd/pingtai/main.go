package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/handler"
	"pingtai/internal/httpapi"
	"pingtai/internal/repository"
	authservice "pingtai/internal/service/auth"
	"pingtai/internal/service/coze"
	"pingtai/internal/service/ctyun"
	"pingtai/internal/service/openai"
	"pingtai/internal/setup"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, startupError := config.Load()
	if len(os.Args) > 1 && os.Args[1] == "create-admin" {
		if startupError != nil {
			return startupError
		}
		return runCreateAdmin(cfg, os.Args[2:])
	}
	if startupError != nil {
		cfg = config.Defaults()
	}
	_, portText, addressError := net.SplitHostPort(cfg.Addr)
	listenPort, portError := strconv.Atoi(portText)
	if addressError != nil || portError != nil || listenPort < 1 || listenPort > 65535 {
		startupError = errors.New("invalid HTTP address; using :5050 for setup")
		cfg.Addr = ":5050"
	}
	if startupError == nil {
		startupError = cfg.Validate()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var app http.Handler
	var configuration *setup.Handler
	if startupError == nil {
		var cleanup func()
		app, configuration, cleanup, startupError = buildApplication(ctx, cfg)
		if cleanup != nil {
			defer cleanup()
		}
	}
	if startupError != nil {
		log.Printf("Application is not ready; starting web setup: %v", startupError)
		configuration = setup.New(cfg, nil)
		app = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/" {
				http.Redirect(w, r, "/setup", http.StatusSeeOther)
				return
			}
			http.Error(w, "Configuration required. Open /setup.", http.StatusServiceUnavailable)
		})
	}

	root := http.NewServeMux()
	root.Handle("/setup", configuration)
	root.Handle("/setup/", configuration)
	root.Handle("/", app)
	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           root,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}
	address := browserAddress(listener.Addr().String())
	if token := configuration.BootstrapToken(); token != "" {
		address += "/setup#token=" + token
		log.Printf("Web setup link (private, valid for this process only): %s", address)
		log.Printf("For remote setup, replace the host with your server address and use HTTPS or an SSH tunnel.")
	} else {
		log.Printf("Platform: %s ; administrator configuration: %s/setup", address, address)
	}
	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("Go platform listening on %s (chat provider: %s)", cfg.Addr, cfg.ChatProvider)
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

func buildApplication(ctx context.Context, cfg config.Config) (http.Handler, *setup.Handler, func(), error) {
	db, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return nil, nil, nil, errors.New("cannot open SQLite database; check the SQLite file path and directory permissions in /setup")
	}
	cleanup := func() { _ = db.Close() }
	if err := database.Migrate(ctx, db); err != nil {
		cleanup()
		return nil, nil, nil, errors.New("SQLite database migration failed; check the database file and directory permissions")
	}
	if !cfg.SSOConfigured() {
		var admins int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_user WHERE usertype=1 AND status=1 AND COALESCE(deltag,0)=0").Scan(&admins); err != nil {
			cleanup()
			return nil, nil, nil, errors.New("cannot read administrator records")
		}
		if admins == 0 {
			cleanup()
			return nil, nil, nil, errors.New("create the first administrator in web setup")
		}
	}
	users := repository.NewUserRepository(db)
	chats := repository.NewChatRepository(db)
	auth := authservice.NewService(users, cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	var modelClient handler.ModelClient = ctyun.NewClient(cfg)
	if cfg.ChatProvider == "openai" {
		modelClient = openai.NewClient(cfg)
	}
	appHandler := handler.New(&cfg, db, users, chats, auth, coze.NewClient(cfg), modelClient)
	return httpapi.NewRouter(&cfg, appHandler), setup.New(cfg, auth), cleanup, nil
}

func browserAddress(address string) string {
	host, port, _ := net.SplitHostPort(address)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}
