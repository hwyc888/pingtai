package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
)

// Optional interoperability test; the Python environment must already contain the official openai SDK.
func TestOfficialOpenAISDK(t *testing.T) {
	python := os.Getenv("PINGTAI_TEST_PYTHON")
	if python == "" {
		t.Skip("set PINGTAI_TEST_PYTHON to a Python executable with the openai package installed")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/v1/models" {
			_, _ = io.WriteString(w, `{"object":"list","data":[{"id":"test-model","object":"model","created":0,"owned_by":"test"}]}`)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		messages := payload["messages"].([]any)
		lastMessage := messages[len(messages)-1].(map[string]any)
		if lastMessage["content"] == "rate-limit" {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"error":{"message":"limited","type":"rate_limit_error","param":null,"code":"rate_limit_exceeded"}}`)
			return
		}
		if payload["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-test\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test-model\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Alice\"},\"finish_reason\":null}]}\n\ndata: [DONE]\n\n")
			return
		}
		if payload["tools"] != nil {
			_, _ = io.WriteString(w, `{"id":"chatcmpl-tool","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_test","type":"function","function":{"name":"weather","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"chatcmpl-test","object":"chat.completion","created":1,"model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"Alice"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":1,"total_tokens":11}}`)
	}))
	defer upstream.Close()
	server := httptest.NewServer(modelRouter(upstream.URL))
	defer server.Close()
	script := `
import sys
from openai import OpenAI, AuthenticationError, RateLimitError

client = OpenAI(base_url=sys.argv[1], api_key="platform-key", max_retries=0, timeout=10)
messages = [
    {"role": "system", "content": "Remember names"},
    {"role": "user", "content": "I am Alice"},
    {"role": "assistant", "content": "Hello Alice"},
    {"role": "user", "content": "What is my name?"},
]
response = client.chat.completions.create(model="test-model", messages=messages)
assert response.choices[0].message.content == "Alice"
assert response.usage.total_tokens == 11
stream = client.chat.completions.create(model="test-model", messages=messages, stream=True)
assert "".join(chunk.choices[0].delta.content or "" for chunk in stream) == "Alice"
assert client.models.list().data[0].id == "test-model"
tools = [{"type": "function", "function": {"name": "weather", "parameters": {"type": "object"}}}]
response = client.chat.completions.create(model="test-model", messages=messages, tools=tools)
assert response.choices[0].message.tool_calls[0].function.name == "weather"
try:
    client.chat.completions.create(model="test-model", messages=[{"role": "user", "content": "rate-limit"}])
    raise AssertionError("expected RateLimitError")
except RateLimitError as error:
    assert error.code == "rate_limit_exceeded"
try:
    OpenAI(base_url=sys.argv[1], api_key="wrong-key", max_retries=0).models.list()
    raise AssertionError("expected AuthenticationError")
except AuthenticationError:
    pass
print("Official OpenAI SDK: completion, history, streaming, models, tools and errors passed")
`
	output, err := exec.Command(python, "-c", script, server.URL+"/v1").CombinedOutput()
	if err != nil {
		t.Fatalf("official SDK interoperability failed: %v\n%s", err, output)
	}
	t.Log(string(output))
}
