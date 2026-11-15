package openai

import (
	"bytes"
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
	cfg    config.Config
	client *http.Client
}

type RequestError struct {
	Message string
	Param   string
}

func (requestError *RequestError) Error() string { return requestError.Message }

func NewClient(cfg config.Config) *Client {
	timeout := cfg.OpenAITimeout
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	return &Client{cfg: cfg, client: &http.Client{
		Timeout: timeout,
		// Do not follow redirects with upstream credentials or convert POSTs into GETs.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment, MaxIdleConns: 100, MaxIdleConnsPerHost: 10,
			IdleConnTimeout: 90 * time.Second,
		},
	}}
}

// Endpoint accepts an API base path or a complete chat-completions URL.
func Endpoint(baseURL, resource string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("Base URL must be an HTTP(S) URL without embedded credentials or a fragment")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(basePath, "/chat/completions") {
		basePath = strings.TrimSuffix(basePath, "/chat/completions")
	}
	if basePath == "" {
		basePath = "/v1"
	}
	parsed.Path = basePath + "/" + resource
	parsed.RawPath = ""
	return parsed.String(), nil
}

func (c *Client) Configured() bool {
	_, err := Endpoint(c.cfg.OpenAIBaseURL, "chat/completions")
	return err == nil
}

func (c *Client) Do(ctx context.Context, body []byte) (*http.Response, error) {
	normalized, err := c.normalizeBody(body)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodPost, "chat/completions", normalized)
}

func (c *Client) Models(ctx context.Context) (*http.Response, error) {
	return c.request(ctx, http.MethodGet, "models", nil)
}

func (c *Client) request(ctx context.Context, method, resource string, body []byte) (*http.Response, error) {
	endpoint, err := Endpoint(c.cfg.OpenAIBaseURL, resource)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if apiKey := strings.TrimSpace(c.cfg.OpenAIAPIKey); apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+apiKey)
	}
	return c.client.Do(request)
}

func (c *Client) Complete(ctx context.Context, payload map[string]any) ([]byte, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	response, err := c.Do(ctx, body)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(io.LimitReader(response.Body, (32<<20)+1))
	if len(contents) > 32<<20 {
		return nil, response.StatusCode, errors.New("upstream response exceeds 32 MB")
	}
	return contents, response.StatusCode, err
}

func (c *Client) normalizeBody(body []byte) ([]byte, error) {
	// RawMessage preserves tool calls, multimodal content, vendor extensions and numeric precision.
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return nil, &RequestError{Message: "Request body must be a JSON object"}
	}
	var messages []json.RawMessage
	if err := json.Unmarshal(payload["messages"], &messages); err != nil || len(messages) == 0 {
		return nil, &RequestError{Message: "messages must be a non-empty array", Param: "messages"}
	}
	for index, message := range messages {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(message, &fields); err != nil || fields == nil {
			return nil, &RequestError{Message: "Each message must be an object", Param: fmt.Sprintf("messages[%d]", index)}
		}
		var role string
		if err := json.Unmarshal(fields["role"], &role); err != nil || strings.TrimSpace(role) == "" {
			return nil, &RequestError{Message: "Each message needs a role", Param: fmt.Sprintf("messages[%d].role", index)}
		}
	}
	var model string
	if rawModel, exists := payload["model"]; exists {
		if err := json.Unmarshal(rawModel, &model); err != nil || string(rawModel) == "null" {
			return nil, &RequestError{Message: "model must be a string", Param: "model"}
		}
	}
	if strings.TrimSpace(model) == "" {
		if strings.TrimSpace(c.cfg.DefaultModel()) == "" {
			return nil, &RequestError{Message: "Provide model or configure a default model", Param: "model"}
		}
		payload["model"], _ = json.Marshal(c.cfg.DefaultModel())
	}
	if rawStream, exists := payload["stream"]; exists {
		var stream bool
		if err := json.Unmarshal(rawStream, &stream); err != nil || string(rawStream) == "null" {
			return nil, &RequestError{Message: "stream must be a boolean", Param: "stream"}
		}
	}
	// API callers control context and generation parameters; never truncate or inject a system message.
	return json.Marshal(payload)
}
