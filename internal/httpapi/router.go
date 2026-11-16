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
	api.HandleFunc("GET /auth/mode", h.AuthMode)
	api.HandleFunc("GET /auth/sso/login", h.SSOLogin)
	api.HandleFunc("GET /auth/sso/callback", h.SSOCallback)
	api.HandleFunc("GET /auth/sso/logout", h.SSOLogout)
	api.Handle("GET /auth/me", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.CurrentUser)))
	api.Handle("POST /auth/refresh", middleware.RequireRefresh(h.Auth.Tokens(), http.HandlerFunc(h.Refresh)))
	api.HandleFunc("GET /auth/logout", h.Logout)
	api.HandleFunc("POST /auth/logout", h.Logout)

	admin := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.SSOConfigured() {
				middleware.RequireAdminAccounts(h.Auth.Tokens(), cfg.SSOAdminAccounts, next).ServeHTTP(w, r)
				return
			}
			middleware.RequireAdmin(h.Auth.Tokens(), h.Users, next).ServeHTTP(w, r)
		})
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

	api.Handle("GET /admin/auth-settings", admin(http.HandlerFunc(h.GetAuthSettings)))
	api.Handle("POST /admin/auth-settings", admin(http.HandlerFunc(h.SaveAuthSettings)))

	api.Handle("GET /admin/knowledge/bases", admin(http.HandlerFunc(h.ListKnowledgeBases)))
	api.Handle("POST /admin/knowledge/bases/create", admin(http.HandlerFunc(h.CreateKnowledgeBase)))
	api.Handle("POST /admin/knowledge/bases/update", admin(http.HandlerFunc(h.UpdateKnowledgeBase)))
	api.Handle("POST /admin/knowledge/bases/delete", admin(http.HandlerFunc(h.DeleteKnowledgeBase)))
	api.Handle("GET /admin/knowledge/documents", admin(http.HandlerFunc(h.ListLocalKnowledgeDocuments)))
	api.Handle("POST /admin/knowledge/documents/upload", admin(http.HandlerFunc(h.UploadLocalKnowledgeDocuments)))
	api.Handle("POST /admin/knowledge/documents/delete", admin(http.HandlerFunc(h.DeleteLocalKnowledgeDocument)))
	api.Handle("GET /admin/knowledge/versions", admin(http.HandlerFunc(h.ListLocalKnowledgeVersions)))
	api.Handle("POST /admin/knowledge/versions/publish", admin(http.HandlerFunc(h.PublishLocalKnowledgeVersion)))

	knowledgeAccess := func(next http.Handler) http.Handler {
		return middleware.RequireAccess(h.Auth.Tokens(), next)
	}
	api.Handle("GET /knowledge/manage/bases", knowledgeAccess(http.HandlerFunc(h.ListKnowledgeBases)))
	api.Handle("POST /knowledge/manage/bases/create", knowledgeAccess(http.HandlerFunc(h.CreateKnowledgeBase)))
	api.Handle("POST /knowledge/manage/bases/update", knowledgeAccess(http.HandlerFunc(h.UpdateKnowledgeBase)))
	api.Handle("POST /knowledge/manage/bases/delete", knowledgeAccess(http.HandlerFunc(h.DeleteKnowledgeBase)))
	api.Handle("GET /knowledge/manage/documents", knowledgeAccess(http.HandlerFunc(h.ListLocalKnowledgeDocuments)))
	api.Handle("POST /knowledge/manage/documents/upload", knowledgeAccess(http.HandlerFunc(h.UploadLocalKnowledgeDocuments)))
	api.Handle("POST /knowledge/manage/documents/delete", knowledgeAccess(http.HandlerFunc(h.DeleteLocalKnowledgeDocument)))
	api.Handle("GET /knowledge/manage/versions", knowledgeAccess(http.HandlerFunc(h.ListLocalKnowledgeVersions)))
	api.Handle("POST /knowledge/manage/versions/publish", knowledgeAccess(http.HandlerFunc(h.PublishLocalKnowledgeVersion)))

	api.Handle("GET /knowledge/bases", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.ListAvailableKnowledgeBases)))
	api.Handle("GET /knowledge/my-bases", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.ListMyKnowledgeBases)))

	api.Handle("GET /agents/models", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.ListChatModels)))
	api.Handle("POST /agents/createConversation", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.CreateConversation)))
	api.Handle("GET /agents/getConversation", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.GetConversation)))
	api.Handle("GET /agents/listConversations", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.ListConversations)))
	api.Handle("POST /agents/deleteConversation", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.DeleteConversation)))
	api.Handle("POST /agents/clearConversation", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.ClearConversation)))
	api.Handle("POST /agents/clearConversations", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.ClearAllConversations)))
	api.Handle("POST /agents/editMessage", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.PrepareMessageEdit)))
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
