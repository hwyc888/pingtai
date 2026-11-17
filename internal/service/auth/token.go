package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Claims struct {
	Subject   string `json:"sub"`
	Name      string `json:"name,omitempty"`
	UserID    string `json:"uid,omitempty"`
	Provider  string `json:"provider,omitempty"`
	Type      string `json:"type"`
	JTI       string `json:"jti"`
	Issued    int64  `json:"iat"`
	NotBefore int64  `json:"nbf"`
	Expires   int64  `json:"exp"`
	Fresh     bool   `json:"fresh"`
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
}

type TokenManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	mu         sync.Mutex
	revoked    map[string]time.Time
}

func NewTokenManager(secret string, accessTTL, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{
		secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL,
		revoked: make(map[string]time.Time),
	}
}

func (m *TokenManager) IssuePair(subject string) (TokenPair, error) {
	return m.IssuePairWithName(subject, "")
}

func (m *TokenManager) IssuePairWithName(subject, name string) (TokenPair, error) {
	return m.IssuePairForIdentity(subject, name, "", "")
}

func (m *TokenManager) IssuePairForIdentity(subject, name, userID, provider string) (TokenPair, error) {
	access, err := m.issue(subject, name, userID, provider, "access", m.accessTTL, true)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := m.issue(subject, name, userID, provider, "refresh", m.refreshTTL, false)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}

func (m *TokenManager) IssueAccess(subject string) (string, error) {
	return m.IssueAccessWithName(subject, "")
}

func (m *TokenManager) IssueAccessWithName(subject, name string) (string, error) {
	return m.IssueAccessForIdentity(subject, name, "", "")
}

func (m *TokenManager) IssueAccessForIdentity(subject, name, userID, provider string) (string, error) {
	return m.issue(subject, name, userID, provider, "access", m.accessTTL, true)
}

func (m *TokenManager) issue(subject, name, userID, provider, tokenType string, ttl time.Duration, fresh bool) (string, error) {
	if subject == "" || len(m.secret) == 0 {
		return "", errors.New("token subject and signing secret are required")
	}
	now := time.Now()
	jti, err := randomTokenID()
	if err != nil {
		return "", err
	}
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claims := Claims{
		Subject: subject, Name: name, UserID: userID, Provider: provider, Type: tokenType, JTI: jti,
		Issued: now.Unix(), NotBefore: now.Unix(),
		Expires: now.Add(ttl).Unix(), Fresh: fresh,
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(unsigned))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return unsigned + "." + signature, nil
}

func (m *TokenManager) Parse(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, errors.New("invalid token")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, errors.New("invalid token header")
	}
	var header struct {
		Algorithm string `json:"alg"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil || header.Algorithm != "HS256" {
		return Claims{}, errors.New("invalid token algorithm")
	}

	unsigned := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(unsigned))
	expected := mac.Sum(nil)
	actual, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(expected, actual) {
		return Claims{}, errors.New("invalid token signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, errors.New("invalid token payload")
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Claims{}, errors.New("invalid token claims")
	}
	now := time.Now().Unix()
	if claims.Subject == "" || claims.JTI == "" || claims.Type == "" || claims.Expires <= now || claims.NotBefore > now {
		return Claims{}, errors.New("token expired or not active")
	}
	if m.IsRevoked(claims.JTI) {
		return Claims{}, errors.New("token revoked")
	}
	return claims, nil
}

func (m *TokenManager) Revoke(jti string, expiresAt int64) {
	if jti == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revoked[jti] = time.Unix(expiresAt, 0)
	m.cleanupLocked(time.Now())
}

func (m *TokenManager) IsRevoked(jti string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked(time.Now())
	_, exists := m.revoked[jti]
	return exists
}

func (m *TokenManager) cleanupLocked(now time.Time) {
	for id, expires := range m.revoked {
		if !expires.After(now) {
			delete(m.revoked, id)
		}
	}
}

func randomTokenID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate token id: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}
