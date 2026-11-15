package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"pingtai/internal/config"
	"pingtai/internal/handler"
	"pingtai/internal/middleware"
)

func NewRouter(cfg *config.Config, h *handler.Handler) http.Handler {
	api := http.NewServeMux()

	api.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := h.DB.PingContext(ctx); err != nil {
			writeHealth(w, http.StatusServiceUnavailable, map[string]any{"status": "degraded", "database": "unavailable"})
			return
		}
		writeHealth(w, http.StatusOK, map[string]any{
			"status":           "ok",
			"database":         "connected",
			"chat_provider":    cfg.ChatProvider,
			"model_configured": h.Model.Configured(),
		})
	})

	api.HandleFunc("GET /auth/bootstrap/status", h.BootstrapStatus)
	api.HandleFunc("POST /auth/bootstrap/register", h.RegisterFirstAdmin)
	api.HandleFunc("POST /auth/login", h.Login)
	api.Handle("GET /auth/me", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.CurrentUser)))
	api.Handle("POST /auth/refresh", middleware.RequireRefresh(h.Auth.Tokens(), http.HandlerFunc(h.Refresh)))
	api.HandleFunc("GET /auth/logout", h.Logout)
	api.HandleFunc("POST /auth/logout", h.Logout)

	admin := func(next http.Handler) http.Handler {
		return middleware.RequireAdmin(h.Auth.Tokens(), h.Users, next)
	}
	api.Handle("POST /auth/addUser", admin(http.HandlerFunc(h.AddUser)))
	api.Handle("POST /auth/delUser", admin(http.HandlerFunc(h.DeleteUser)))
	api.Handle("GET /auth/get_user_list", admin(http.HandlerFunc(h.GetUserList)))
	api.Handle("POST /auth/editUser", admin(http.HandlerFunc(h.EditUser)))

	api.Handle("GET /admin/providers", admin(http.HandlerFunc(h.ListProviders)))
	api.Handle("POST /admin/providers/save", admin(http.HandlerFunc(h.SaveProvider)))
	api.Handle("POST /admin/providers/delete", admin(http.HandlerFunc(h.DeleteProvider)))
	api.Handle("POST /admin/providers/activate", admin(http.HandlerFunc(h.ActivateProvider)))
	api.Handle("POST /admin/providers/discover", admin(http.HandlerFunc(h.DiscoverProviderModels)))
	api.Handle("POST /admin/providers/models/update", admin(http.HandlerFunc(h.UpdateProviderModel)))
	api.Handle("POST /admin/providers/test", admin(http.HandlerFunc(h.TestProviderConnection)))
	api.Handle("POST /admin/providers/test-model", admin(http.HandlerFunc(h.TestProviderModel)))

	api.Handle("GET /agents/models", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.ListChatModels)))
	api.Handle("POST /agents/createConversation", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.CreateConversation)))
	api.Handle("GET /agents/getConversation", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.GetConversation)))
	api.Handle("GET /agents/listConversations", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.ListConversations)))
	api.Handle("POST /agents/deleteConversation", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.DeleteConversation)))
	api.Handle("POST /agents/chat", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.Chat)))
	api.Handle("GET /agents/getChatDetail", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.GetChatDetail)))
	api.Handle("GET /agents/getChatMessage", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.GetChatMessage)))
	api.Handle("POST /agents/getMessageList", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.GetMessageList)))
	api.Handle("GET /agents/getMessage", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.GetMessage)))

	modelAPI := middleware.RequireAPIOrAccess(h.Auth.Tokens(), cfg.ModelAPIKey, http.HandlerFunc(h.ChatCompletions))
	api.Handle("POST /v1/chat/completions", modelAPI)
	api.Handle("POST /model/chat/completions", modelAPI)
	api.Handle("GET /v1/models", middleware.RequireAPIOrAccess(h.Auth.Tokens(), cfg.ModelAPIKey, http.HandlerFunc(h.ListModels)))

	root := http.NewServeMux()
	root.Handle("/api/", http.StripPrefix("/api", api))
	root.Handle("/auth/", api)
	root.Handle("/agents/", api)
	root.Handle("/admin/providers", api)
	root.Handle("/admin/providers/", api)
	root.Handle("/admin", spaFiles(cfg.WebDir))
	root.Handle("/admin/", spaFiles(cfg.WebDir))
	root.Handle("/model/", api)
	root.Handle("/v1/", api)
	root.Handle("/health", api)
	root.Handle("/", spaFiles(cfg.WebDir))
	return middleware.CORS(root)
}

func writeHealth(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
