package ctyun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"pingtai/internal/config"
)

type Client struct {
	cfg    config.Config
	client *http.Client
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		cfg: cfg,
		client: &http.Client{
			Timeout: ctyunTimeout(cfg),
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func ctyunTimeout(cfg config.Config) time.Duration {
	if cfg.CTYunTimeout <= 0 {
		return 180 * time.Second
	}
	return cfg.CTYunTimeout
}

func (c *Client) Configured() bool {
	return c.cfg.CTYunConfigured()
}

func (c *Client) Do(ctx context.Context, body []byte) (*http.Response, error) {
	if !c.Configured() {
		return nil, errors.New("CTYun API is not configured; check CTYUN_API_URL and authentication settings")
	}
	normalized, err := c.normalizeBody(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.CTYunAPIURL, bytes.NewReader(normalized))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	if c.cfg.CTYunAuthMode == "signature" {
		headers, err := BuildSignatureHeaders(c.cfg.CTYunAPIURL, c.cfg, normalized, time.Now())
		if err != nil {
			return nil, fmt.Errorf("create CTYun signature: %w", err)
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}
	} else {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.cfg.CTYunAPIKey))
	}
	return c.client.Do(req)
}

func (c *Client) Complete(ctx context.Context, payload map[string]any) ([]byte, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.Do(ctx, body)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	result, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return result, resp.StatusCode, err
}

func (c *Client) normalizeBody(body []byte) ([]byte, error) {
	payload := make(map[string]any)
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("request body must be a JSON object: %w", err)
	}
	if model, ok := payload["model"].(string); !ok || strings.TrimSpace(model) == "" {
		if c.cfg.CTYunModel == "" {
			return nil, errors.New("model is required; set CTYUN_MODEL or include model in request")
		}
		payload["model"] = c.cfg.CTYunModel
	}
	if _, exists := payload["max_completion_tokens"]; !exists {
		if _, hasLegacyLimit := payload["max_tokens"]; !hasLegacyLimit && c.cfg.CTYunMaxTokens > 0 {
			payload["max_completion_tokens"] = c.cfg.CTYunMaxTokens
		}
	}
	if _, exists := payload["knowledgeBase"]; !exists && c.cfg.CTYunKnowledgeBase != "" {
		payload["knowledgeBase"] = c.cfg.CTYunKnowledgeBase
	}
	if c.cfg.CTYunAuthMode == "signature" {
		if _, exists := payload["traceId"]; !exists {
			payload["traceId"] = newTraceID()
		}
		if _, exists := payload["timestamp"]; !exists {
			payload["timestamp"] = time.Now().Unix()
		}
	}
	return json.Marshal(payload)
}

func newTraceID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano())
}
