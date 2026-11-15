package httpapi

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pingtai/internal/config"
	"pingtai/internal/handler"
	authservice "pingtai/internal/service/auth"
	"pingtai/internal/service/openai"
)

func modelRouter(upstreamURL string) http.Handler {
	cfg := config.Defaults()
	cfg.ModelAPIKey = "platform-key"
	cfg.OpenAIBaseURL = upstreamURL + "/v1"
	cfg.OpenAIAPIKey = "upstream-key"
	cfg.OpenAIModel = "default-model"
	auth := authservice.NewService(nil, strings.Repeat("secret", 8), time.Minute, time.Hour)
	application := &handler.Handler{Config: &cfg, Auth: auth, Model: openai.NewClient(cfg)}
	return NewRouter(&cfg, application)
}

func TestModelRoutesErrorsAndModels(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer upstream-key" {
			t.Error("platform key leaked upstream or upstream key missing")
		}
		if request.URL.Path == "/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"test-model","object":"model","created":0,"owned_by":"test"}]}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"slow down","type":"rate_limit_error","code":"rate_limit_exceeded","param":null}}`)
	}))
	defer upstream.Close()
	router := modelRouter(upstream.URL)
	for _, path := range []string{"/v1/chat/completions", "/api/v1/chat/completions", "/model/chat/completions"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"messages":[{"role":"user","content":"hello"}]}`))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || !strings.Contains(response.Body.String(), `"error"`) {
			t.Fatalf("invalid key error at %s: %d %s", path, response.Code, response.Body.String())
		}
		request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"messages":[]}`))
		request.Header.Set("Authorization", "Bearer platform-key")
		response = httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"param":"messages"`) {
			t.Fatalf("invalid request at %s: %d %s", path, response.Code, response.Body.String())
		}
		request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"messages":[{"role":"user","content":"hello"}]}`))
		request.Header.Set("Authorization", "Bearer platform-key")
		response = httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusTooManyRequests || response.Header().Get("Retry-After") != "5" || !strings.Contains(response.Body.String(), "rate_limit_exceeded") {
			t.Fatalf("upstream error was changed: %d %s", response.Code, response.Body.String())
		}
	}
	for _, path := range []string{"/v1/models", "/api/v1/models"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("X-API-Key", "platform-key")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "test-model") {
			t.Fatalf("model listing failed: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestStreamingFlushesBeforeUpstreamFinishes(t *testing.T) {
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-request.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	server := httptest.NewServer(modelRouter(upstream.URL))
	defer func() {
		close(release)
		server.Close()
		upstream.Close()
	}()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"stream":true,"messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer platform-key")
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Header.Get("X-Accel-Buffering") != "no" || response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("SSE headers were not preserved")
	}
	line, err := bufio.NewReader(response.Body).ReadString('\n')
	if err != nil || !strings.Contains(line, "first") {
		t.Fatalf("first event was buffered until completion: %q %v", line, err)
	}
	var event map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event); err != nil {
		t.Fatal(err)
	}
}
