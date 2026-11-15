package sso

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pingtai/internal/config"
)

type Client struct {
	baseURL      string
	clientID     string
	clientSecret string
	redirectURI  string
	httpClient   *http.Client
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type UserProfile struct {
	Account     string
	DisplayName string
	ID          string
	Phone       string
	Email       string
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		baseURL:      strings.TrimRight(strings.TrimSpace(cfg.SSOBaseURL), "/"),
		clientID:     strings.TrimSpace(cfg.SSOClientID),
		clientSecret: strings.TrimSpace(cfg.SSOClientSecret),
		redirectURI:  strings.TrimSpace(cfg.SSORedirectURL),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *Client) AuthorizationURL(state string) (string, error) {
	if c.baseURL == "" || c.clientID == "" || c.redirectURI == "" || state == "" {
		return "", errors.New("SSO client is not configured")
	}
	endpoint, err := url.Parse(c.baseURL + "/oauth2.0/authorize")
	if err != nil {
		return "", errors.New("invalid SSO authorization URL")
	}
	query := endpoint.Query()
	query.Set("response_type", "code")
	query.Set("client_id", c.clientID)
	query.Set("redirect_uri", c.redirectURI)
	query.Set("state", state)
	endpoint.RawQuery = query.Encode()
	return endpoint.String(), nil
}

func (c *Client) ExchangeCode(ctx context.Context, code string) (TokenResponse, error) {
	if c.baseURL == "" || c.clientID == "" || c.clientSecret == "" || c.redirectURI == "" {
		return TokenResponse{}, errors.New("SSO client is not configured")
	}
	if strings.TrimSpace(code) == "" {
		return TokenResponse{}, errors.New("authorization code is empty")
	}
	form := url.Values{}
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", c.redirectURI)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/oauth2.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return TokenResponse{}, errors.New("cannot create SSO token request")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return TokenResponse{}, errors.New("cannot reach the SSO token endpoint")
	}
	defer response.Body.Close()
	body, err := readLimited(response.Body)
	if err != nil {
		return TokenResponse{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return TokenResponse{}, fmt.Errorf("SSO token endpoint returned HTTP %d", response.StatusCode)
	}
	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return TokenResponse{}, errors.New("SSO token response is not valid JSON")
	}
	if token.AccessToken == "" {
		return TokenResponse{}, errors.New("SSO token response did not include access_token")
	}
	if token.TokenType != "" && !strings.EqualFold(token.TokenType, "bearer") {
		return TokenResponse{}, errors.New("SSO returned an unsupported token type")
	}
	return token, nil
}

func (c *Client) UserProfile(ctx context.Context, accessToken string) (UserProfile, error) {
	if strings.TrimSpace(accessToken) == "" {
		return UserProfile{}, errors.New("SSO access token is empty")
	}
	endpoint, err := url.Parse(c.baseURL + "/oauth2.0/profile")
	if err != nil {
		return UserProfile{}, errors.New("invalid SSO profile URL")
	}
	query := endpoint.Query()
	query.Set("access_token", accessToken)
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return UserProfile{}, errors.New("cannot create SSO profile request")
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return UserProfile{}, errors.New("cannot reach the SSO profile endpoint")
	}
	defer response.Body.Close()
	body, err := readLimited(response.Body)
	if err != nil {
		return UserProfile{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return UserProfile{}, fmt.Errorf("SSO profile endpoint returned HTTP %d", response.StatusCode)
	}
	profile, err := ParseUserProfile(body)
	if err != nil {
		return UserProfile{}, err
	}
	return profile, nil
}

func (c *Client) RefreshToken(ctx context.Context, refreshToken string) (TokenResponse, error) {
	if c.baseURL == "" || c.clientID == "" || c.clientSecret == "" {
		return TokenResponse{}, errors.New("SSO client is not configured")
	}
	if strings.TrimSpace(refreshToken) == "" {
		return TokenResponse{}, errors.New("SSO refresh token is empty")
	}
	form := url.Values{}
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/oauth2.0/token", strings.NewReader(form.Encode()))
	if err != nil {
		return TokenResponse{}, errors.New("cannot create SSO refresh request")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return TokenResponse{}, errors.New("cannot reach the SSO token endpoint")
	}
	defer response.Body.Close()
	body, err := readLimited(response.Body)
	if err != nil {
		return TokenResponse{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return TokenResponse{}, fmt.Errorf("SSO token refresh returned HTTP %d", response.StatusCode)
	}
	var token TokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return TokenResponse{}, errors.New("SSO refresh response is not valid JSON")
	}
	if token.AccessToken == "" {
		return TokenResponse{}, errors.New("SSO refresh response did not include access_token")
	}
	return token, nil
}

func ParseUserProfile(body []byte) (UserProfile, error) {
	var payload struct {
		ID          string `json:"id"`
		Account     string `json:"account"`
		UserAccount string `json:"useraccount"`
		Username    string `json:"username"`
		Phone       string `json:"phone"`
		Email       string `json:"email"`
		User        *struct {
			ID          string `json:"id"`
			Account     string `json:"account"`
			UserAccount string `json:"useraccount"`
			Username    string `json:"username"`
			Phone       string `json:"phone"`
			Email       string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return UserProfile{}, errors.New("SSO profile response is not valid JSON")
	}
	profile := UserProfile{
		ID:          strings.TrimSpace(payload.ID),
		Account:     firstNonEmpty(payload.UserAccount, payload.Account, payload.ID),
		DisplayName: strings.TrimSpace(payload.Username),
		Phone:       strings.TrimSpace(payload.Phone),
		Email:       strings.TrimSpace(payload.Email),
	}
	if payload.User != nil {
		profile.Account = firstNonEmpty(payload.User.UserAccount, payload.User.Account, profile.Account, payload.User.ID)
		profile.DisplayName = firstNonEmpty(payload.User.Username, profile.DisplayName)
		profile.Phone = firstNonEmpty(payload.User.Phone, profile.Phone)
		profile.Email = firstNonEmpty(payload.User.Email, profile.Email)
		profile.ID = firstNonEmpty(payload.User.ID, profile.ID)
	}
	if profile.Account == "" {
		return UserProfile{}, errors.New("SSO profile did not include a usable account identifier")
	}
	if profile.DisplayName == "" {
		profile.DisplayName = profile.Account
	}
	return profile, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func readLimited(reader io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, (1<<20)+1))
	if err != nil {
		return nil, errors.New("cannot read SSO response")
	}
	if len(body) > 1<<20 {
		return nil, errors.New("SSO response exceeded 1 MB")
	}
	return body, nil
}
