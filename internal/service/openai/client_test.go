package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"pingtai/internal/config"
)

func TestEndpointAcceptsCommonBaseURLFormats(t *testing.T) {
	for _, test := range []struct{ base, expected string }{
		{"https://provider.example", "https://provider.example/v1/chat/completions"},
		{"https://provider.example/v1/", "https://provider.example/v1/chat/completions"},
		{"https://provider.example/compatible-mode/v1", "https://provider.example/compatible-mode/v1/chat/completions"},
		{"https://provider.example/api/v3/chat/completions?version=test", "https://provider.example/api/v3/chat/completions?version=test"},
	} {
		actual, err := Endpoint(test.base, "chat/completions")
		if err != nil || actual != test.expected {
			t.Fatalf("base=%s endpoint=%s err=%v", test.base, actual, err)
		}
	}
	actual, err := Endpoint("https://provider.example/api/v3/chat/completions", "models")
	if err != nil || actual != "https://provider.example/api/v3/models" {
		t.Fatalf("unexpected models endpoint: %s %v", actual, err)
	}
}

func TestClientPreservesFullContextToolsMultimodalAndVendorParameters(t *testing.T) {
	body := []byte(`{
		"model":"requested-model",
		"messages":[
			{"role":"developer","content":"Follow instructions"},
			{"role":"user","content":[{"type":"text","text":"remember this image"},{"type":"image_url","image_url":{"url":"https://image.example/a.png","detail":"low"}}]},
			{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"weather","arguments":"{}"}}]},
			{"role":"tool","tool_call_id":"call_1","content":"sunny"},
			{"role":"user","content":"what did the tool say?"}
		],
		"tools":[{"type":"function","function":{"name":"weather","parameters":{"type":"object"}}}],
		"tool_choice":"auto","max_completion_tokens":100,"temperature":0.2,
		"response_format":{"type":"json_object"},"seed":9007199254740993,
		"stream":true,"stream_options":{"include_usage":true},"vendor_extension":{"enable_thinking":false}
	}`)
	var expected map[string]json.RawMessage
	if err := json.Unmarshal(body, &expected); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/compatible-mode/v1/chat/completions" || request.Header.Get("Authorization") != "Bearer upstream-secret" {
			t.Errorf("incorrect upstream path or credentials")
		}
		var actual map[string]json.RawMessage
		if err := json.NewDecoder(request.Body).Decode(&actual); err != nil {
			t.Error(err)
		}
		if len(actual) != len(expected) {
			t.Errorf("parameters were added or removed: %v", actual)
		}
		for key, expectedValue := range expected {
			var expectedJSON, actualJSON any
			// Decode into interface only for structural comparison; verify large integers as raw JSON separately.
			_ = json.Unmarshal(expectedValue, &expectedJSON)
			_ = json.Unmarshal(actual[key], &actualJSON)
			if !reflect.DeepEqual(expectedJSON, actualJSON) {
				t.Errorf("parameter %s changed", key)
			}
		}
		if string(actual["seed"]) != "9007199254740993" {
			t.Error("large integer lost precision")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	cfg := config.Defaults()
	cfg.OpenAIBaseURL = server.URL + "/compatible-mode/v1"
	cfg.OpenAIAPIKey = "upstream-secret"
	cfg.OpenAIModel = "default-must-not-override"
	cfg.ContextTurns = 1
	cfg.OpenAISystemPrompt = "must-not-inject"
	cfg.OpenAIMaxTokens = 1000
	response, err := NewClient(cfg).Do(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal(response.StatusCode)
	}
}

func TestClientDefaultsAndInvalidRequests(t *testing.T) {
	cfg := config.Defaults()
	cfg.OpenAIModel = "default-model"
	client := NewClient(cfg)
	normalized, err := client.normalizeBody([]byte(`{"messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]json.RawMessage
	_ = json.Unmarshal(normalized, &result)
	if string(result["model"]) != `"default-model"` || len(result) != 2 {
		t.Fatalf("unexpected injected defaults: %s", normalized)
	}
	for _, invalid := range []string{
		`null`, `[]`, `{}`, `{"messages":[]}`, `{"messages":[null]}`,
		`{"messages":[{"role":123}]}`, `{"messages":[{"role":"user"}],"model":123}`,
		`{"messages":[{"role":"user"}],"stream":"true"}`,
	} {
		_, err := client.normalizeBody([]byte(invalid))
		var requestError *RequestError
		if !errors.As(err, &requestError) {
			t.Errorf("invalid request was not classified correctly: %s, %v", invalid, err)
		}
	}
}
