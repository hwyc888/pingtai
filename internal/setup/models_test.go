package setup

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pingtai/internal/config"
)

func TestModelDiscoveryUsesUnsavedValuesWithoutSaving(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-created.json")
	t.Setenv("PINGTAI_CONFIG_FILE", path)
	var upstreamKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/custom/v1/models" {
			t.Errorf("unexpected discovery request: %s %s", request.Method, request.URL.Path)
		}
		upstreamKey = request.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"data":[{"id":"model-b"},{"id":"model-a"},{"id":"model-b"},{"id":""}]}`)
	}))
	defer upstream.Close()
	current := config.Defaults()
	current.OpenAIAPIKey = "saved-secret"
	current.OpenAIModels = []string{"previous-model"}
	handler := New(current, nil)
	for _, test := range []struct {
		key      string
		clear    bool
		expected string
	}{
		{key: "new-secret", expected: "Bearer new-secret"},
		{expected: "Bearer saved-secret"},
		{clear: true, expected: ""},
	} {
		body, _ := json.Marshal(map[string]any{"BaseURL": upstream.URL + "/custom/v1/chat/completions", "APIKey": test.key, "ClearAPIKey": test.clear})
		request := httptest.NewRequest(http.MethodPost, "/setup/models", strings.NewReader(string(body)))
		request.Header.Set("X-Setup-Request", "1")
		request.Header.Set("X-Setup-Token", handler.BootstrapToken())
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		var result struct{ Models []string }
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK || upstreamKey != test.expected || !reflect.DeepEqual(result.Models, []string{"model-a", "model-b"}) {
			t.Fatalf("discovery failed: %d %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "secret") {
			t.Fatal("upstream credentials leaked into discovery response")
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) || handler.pending || !reflect.DeepEqual(handler.current, current) {
		t.Fatal("discovery saved or mutated the current configuration")
	}
}

func TestModelDiscoveryRequiresAuthorizationAndPreservesUpstreamFailure(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		called = true
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"secret-do-not-reflect"}}`)
	}))
	defer upstream.Close()
	handler := New(config.Defaults(), nil)
	body := `{"BaseURL":"` + upstream.URL + `/v1"}`
	for _, test := range []struct {
		token, origin string
		status        int
	}{
		{token: "wrong", status: http.StatusUnauthorized},
		{token: handler.BootstrapToken(), origin: "https://attacker.example", status: http.StatusForbidden},
		{token: handler.BootstrapToken(), status: http.StatusBadGateway},
	} {
		request := httptest.NewRequest(http.MethodPost, "/setup/models", strings.NewReader(body))
		request.Header.Set("X-Setup-Request", "1")
		request.Header.Set("X-Setup-Token", test.token)
		request.Header.Set("Origin", test.origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status || strings.Contains(response.Body.String(), "secret-do-not-reflect") {
			t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
		}
		if test.status != http.StatusBadGateway && called {
			t.Fatal("unauthorized discovery reached the upstream")
		}
	}
}
