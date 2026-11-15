package handler

import (
	"context"
	"database/sql"
	"net/http"

	"pingtai/internal/config"
	"pingtai/internal/repository"
	authservice "pingtai/internal/service/auth"
	"pingtai/internal/service/coze"
)

type ModelClient interface {
	Configured() bool
	Do(context.Context, []byte) (*http.Response, error)
	Complete(context.Context, map[string]any) ([]byte, int, error)
}

type Handler struct {
	Config *config.Config
	DB     *sql.DB
	Users  *repository.UserRepository
	Chats  *repository.ChatRepository
	Auth   *authservice.Service
	Coze   *coze.Client
	Model  ModelClient
}

func New(cfg *config.Config, db *sql.DB, users *repository.UserRepository, chats *repository.ChatRepository,
	auth *authservice.Service, cozeClient *coze.Client, modelClient ModelClient) *Handler {
	return &Handler{
		Config: cfg, DB: db, Users: users, Chats: chats, Auth: auth,
		Coze: cozeClient, Model: modelClient,
	}
}
