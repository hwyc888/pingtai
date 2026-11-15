package ctyun

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pingtai/internal/config"
)

func TestBearerClientAddsModelAndForwardsMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-app-key" {
			t.Errorf("unexpected Authorization header: %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if payload["model"] != "test-model" {
			t.Errorf("expected default model, got %#v", payload["model"])
		}
		if payload["max_completion_tokens"] != float64(256) {
			t.Errorf("expected max_completion_tokens=256, got %#v", payload["max_completion_tokens"])
		}
		if payload["traceId"] != nil || payload["timestamp"] != nil {
			t.Errorf("generic Bearer endpoint should not receive Star MaaS-only fields")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()

	cfg := config.Config{
		CTYunAPIURL: server.URL, CTYunAuthMode: "bearer",
		CTYunAPIKey: "test-app-key", CTYunModel: "test-model",
		CTYunMaxTokens: 256, CTYunTimeout: time.Second,
	}
	client := NewClient(cfg)
	body, status, err := client.Complete(context.Background(), map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", status, body)
	}
	if len(body) == 0 {
		t.Fatal("expected response body")
	}
}

func TestSignatureClientAddsRequiredHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-APP-ID") != "app-id" {
			t.Errorf("missing X-APP-ID")
		}
		if r.Header.Get("Order-Num") != "order-1" {
			t.Errorf("missing Order-Num")
		}
		if r.Header.Get("Authorization") == "" {
			t.Errorf("missing signature Authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"signed"}}]}`)
	}))
	defer server.Close()

	cfg := config.Config{
		CTYunAPIURL: server.URL, CTYunAuthMode: "signature",
		CTYunAppID: "app-id", CTYunAppKey: "app-key", CTYunRegion: "QG",
		CTYunSignPrefix: "teleai-cloud-auth-v1", CTYunOrderNum: "order-1",
		CTYunModel: "telechat-115b", CTYunTimeout: time.Second,
	}
	client := NewClient(cfg)
	body, status, err := client.Complete(context.Background(), map[string]any{
		"messages": []any{map[string]any{"role": "user", "content": "hello"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || len(body) == 0 {
		t.Fatalf("unexpected upstream response status=%d body=%s", status, body)
	}
}
