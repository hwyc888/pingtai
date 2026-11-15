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
			"ctyun_configured": cfg.CTYunConfigured(),
			"model_configured": h.Model.Configured(),
			"coze_configured":  h.Coze.Configured(),
		})
	})

	api.HandleFunc("POST /auth/login", h.Login)
	api.Handle("POST /auth/refresh", middleware.RequireRefresh(h.Auth.Tokens(), http.HandlerFunc(h.Refresh)))
	api.Handle("GET /auth/logout", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.Logout)))
	api.Handle("POST /auth/addUser", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.AddUser)))
	api.Handle("POST /auth/delUser", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.DeleteUser)))
	api.Handle("GET /auth/get_user_list", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.GetUserList)))
	api.Handle("POST /auth/editUser", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.EditUser)))

	api.Handle("GET /agents/getAgentConfig", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.GetAgentConfig)))
	api.Handle("POST /agents/updateAgent", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.UpdateAgent)))
	api.Handle("POST /agents/publishAgent", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.PublishAgent)))
	api.Handle("GET /agents/getAgentsList", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.GetAgentsList)))
	api.Handle("POST /agents/createConversation", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.CreateConversation)))
	api.Handle("GET /agents/getConversation", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.GetConversation)))
	api.Handle("POST /agents/chat", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.Chat)))
	api.Handle("GET /agents/getChatDetail", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.GetChatDetail)))
	api.Handle("GET /agents/getChatMessage", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.GetChatMessage)))
	api.Handle("POST /agents/createMessage", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.CreateMessage)))
	api.Handle("POST /agents/getMessageList", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.GetMessageList)))
	api.Handle("GET /agents/getMessage", middleware.RequireAccess(h.Auth.Tokens(), http.HandlerFunc(h.GetMessage)))

	api.Handle("GET /datasets/workspacesList", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.WorkspacesList)))
	api.Handle("POST /datasets/createKnowledge", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.CreateKnowledge)))
	api.Handle("GET /datasets/knowledgeList", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.KnowledgeList)))
	api.Handle("POST /datasets/delKnowledge", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.DeleteKnowledge)))
	api.Handle("POST /datasets/editKnowledge", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.EditKnowledge)))
	api.Handle("POST /datasets/knowledgeFileList", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.KnowledgeFileList)))
	api.Handle("POST /datasets/createKnowledgeFile", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.CreateKnowledgeFile)))
	api.Handle("POST /datasets/deleteKnowledgeFile", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.DeleteKnowledgeFile)))
	api.Handle("POST /datasets/getKnowledgeFileProcess", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.GetKnowledgeFileProcess)))
	api.Handle("POST /datasets/updateKnowledgeFile", middleware.RequireAdmin(h.Auth.Tokens(), h.Users, http.HandlerFunc(h.UpdateKnowledgeFile)))

	modelAPI := middleware.RequireAPIOrAccess(h.Auth.Tokens(), cfg.ModelAPIKey, http.HandlerFunc(h.ChatCompletions))
	api.Handle("POST /v1/chat/completions", modelAPI)
	api.Handle("POST /model/chat/completions", modelAPI)
	api.Handle("GET /v1/models", middleware.RequireAPIOrAccess(h.Auth.Tokens(), cfg.ModelAPIKey, http.HandlerFunc(h.ListModels)))

	root := http.NewServeMux()
	root.Handle("/api/", http.StripPrefix("/api", api))
	root.Handle("/auth/", api)
	root.Handle("/agents/", api)
	root.Handle("/datasets/", api)
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
