package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"pingtai/internal/model"
	"pingtai/internal/repository"
)

var ErrInvalidCredentials = errors.New("invalid username or password")

type Service struct {
	users  *repository.UserRepository
	tokens *TokenManager
}

func NewService(users *repository.UserRepository, secret string, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{
		users:  users,
		tokens: NewTokenManager(secret, accessTTL, refreshTTL),
	}
}

func (s *Service) Tokens() *TokenManager {
	return s.tokens
}

func (s *Service) Authenticate(ctx context.Context, username, password string) (model.User, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return model.User{}, ErrInvalidCredentials
	}
	user, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.User{}, ErrInvalidCredentials
		}
		return model.User{}, err
	}
	if user.DelTag != 0 || user.Status != 1 {
		return model.User{}, ErrInvalidCredentials
	}
	valid, upgrade := VerifyPassword(user.Password, password)
	if !valid {
		return model.User{}, ErrInvalidCredentials
	}
	if upgrade {
		if hash, hashErr := HashPassword(password); hashErr == nil {
			_ = s.users.UpdatePassword(ctx, user.ID, hash)
		}
	}
	return user, nil
}

func (s *Service) IssuePair(username string) (TokenPair, error) {
	return s.tokens.IssuePair(username)
}

func (s *Service) IssuePairWithName(username, name string) (TokenPair, error) {
	return s.tokens.IssuePairWithName(username, name)
}

func (s *Service) IssuePairForIdentity(username, name, userID, provider string) (TokenPair, error) {
	return s.tokens.IssuePairForIdentity(username, name, userID, provider)
}

func (s *Service) IssueAccess(username string) (string, error) {
	return s.tokens.IssueAccess(username)
}

func (s *Service) IssueAccessWithName(username, name string) (string, error) {
	return s.tokens.IssueAccessWithName(username, name)
}

func (s *Service) IssueAccessForIdentity(username, name, userID, provider string) (string, error) {
	return s.tokens.IssueAccessForIdentity(username, name, userID, provider)
}

func (s *Service) Parse(token string) (Claims, error) {
	return s.tokens.Parse(token)
}

func (s *Service) Revoke(claims Claims) {
	s.tokens.Revoke(claims.JTI, claims.Expires)
}
