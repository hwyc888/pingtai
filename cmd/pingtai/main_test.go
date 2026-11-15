package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pingtai/internal/config"
	"pingtai/internal/setup"
)

// This integration test uses an isolated SQLite file and a local mock model provider.
func TestWebSetupRestartIntegration(t *testing.T) {
	sqlitePath := filepath.Join(t.TempDir(), "data", "pingtai.db")
	t.Setenv("PINGTAI_DB_PATH", filepath.Join(t.TempDir(), "stale-environment.db"))
	t.Setenv("PINGTAI_CONFIG_FILE", filepath.Join(t.TempDir(), "pingtai-config.json"))

	upstreamRequests := make(chan map[string]any, 16)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer verification-upstream-key" {
			t.Error("model credentials were not restored after restart")
		}
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected upstream URL: %s", request.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		upstreamRequests <- payload
		w.Header().Set("Content-Type", "application/json")
		messages, _ := payload["messages"].([]any)
		if len(messages) > 0 && messages[len(messages)-1].(map[string]any)["content"] == "simulate-failure" {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `{"error":{"message":"simulated failure"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"restart-ok"}}]}`)
	}))
	defer upstream.Close()
	requestBody := map[string]any{
		"Settings": map[string]any{
			"Addr": ":5051", "OpenAIBaseURL": upstream.URL + "/v1", "OpenAIAPIKey": "verification-upstream-key",
			"OpenAIModel": "verification-model", "ModelAPIKey": "verification-proxy-key",
			"OpenAIModels":       []string{"verification-model", "second-model"},
			"OpenAISystemPrompt": "remember the conversation", "ContextTurns": 1, "DatabasePath": sqlitePath,
		},
		"AdminUsername": "setupadmin", "AdminPassword": "verification-admin-password",
	}
	initial := config.Defaults()
	initial.DatabasePath = sqlitePath
	bootstrap := setup.New(initial, nil)
	response := sendSetupRequest(t, bootstrap, requestBody, bootstrap.BootstrapToken(), "", "")
	if response.Code != http.StatusOK {
		t.Fatalf("initial web save: %d %s", response.Code, response.Body.String())
	}
	// Simulate a fresh process reading saved configuration in the presence of stale environment values.
	t.Setenv("PINGTAI_DB_PATH", filepath.Join(t.TempDir(), "invalid-environment.db"))
	t.Setenv("JWT_SECRET_KEY", "invalid-environment-secret")
	saved, err := config.Load()
	if err != nil || saved.Addr != ":5051" || len(saved.JWTSecret) < 32 {
		t.Fatalf("saved configuration did not survive reload: %v", err)
	}
	application, management, cleanup, err := buildApplication(context.Background(), saved)
	if err != nil {
		t.Fatalf("application did not start after setup: %v", err)
	}
	defer cleanup()
	if management.BootstrapToken() != "" {
		t.Fatal("bootstrap access remains enabled after normal startup")
	}
	health := httptest.NewRecorder()
	application.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health after restart: %d", health.Code)
	}
	var accessToken string
	login := func(password string) int {
		form := url.Values{"username": {"setupadmin"}, "password": {password}}
		request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		application.ServeHTTP(response, request)
		var result struct {
			Errcode     int
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.AccessToken != "" {
			accessToken = result.AccessToken
			cookies := response.Result().Cookies()
			foundAccess, foundRefresh, foundSignedIn := false, false, false
			for _, cookie := range cookies {
				switch cookie.Name {
				case "Authorization":
					foundAccess = cookie.HttpOnly && cookie.Value == result.AccessToken
				case "RefreshToken":
					foundRefresh = cookie.HttpOnly && cookie.Value != ""
				case "pingtai_signed_in":
					foundSignedIn = cookie.Value == "1"
				}
			}
			if !foundAccess || !foundRefresh || !foundSignedIn {
				t.Fatal("local login did not set HttpOnly session cookies and SPA signed-in flag")
			}
		}
		return result.Errcode
	}
	if login("verification-admin-password") != 200 {
		t.Fatal("the administrator created by web setup cannot log in after restart")
	}
	listRequest := httptest.NewRequest(http.MethodGet, "/api/agents/getAgentsList?space_id=openai-workspace", nil)
	listRequest.Header.Set("Authorization", "Bearer "+accessToken)
	listResponse := httptest.NewRecorder()
	application.ServeHTTP(listResponse, listRequest)
	var modelList struct {
		Data struct {
			Bots []struct {
				BotID string `json:"bot_id"`
				Name  string `json:"name"`
			} `json:"space_bots"`
		}
	}
	if err := json.Unmarshal(listResponse.Body.Bytes(), &modelList); err != nil || len(modelList.Data.Bots) != 1 || modelList.Data.Bots[0].Name != "verification-model" {
		t.Fatalf("platform default model was not restored as the only web chat option: %s", listResponse.Body.String())
	}
	modelRequest := httptest.NewRequest(http.MethodPost, "/api/v1/chat/completions", strings.NewReader(`{"messages":[{"role":"user","content":"hello"}]}`))
	modelRequest.Header.Set("Content-Type", "application/json")
	modelRequest.Header.Set("X-API-Key", "verification-proxy-key")
	modelResponse := httptest.NewRecorder()
	application.ServeHTTP(modelResponse, modelRequest)
	if modelResponse.Code != http.StatusOK || !strings.Contains(modelResponse.Body.String(), "restart-ok") {
		t.Fatalf("model proxy after restart: %d %s", modelResponse.Code, modelResponse.Body.String())
	}
	<-upstreamRequests
	createConversation := func() string {
		result := authenticatedRequest(t, application, accessToken, "/api/agents/createConversation", "application/json", `{"bot_id":"openai-default"}`)
		var conversation struct {
			Data struct{ ID string }
		}
		if err := json.Unmarshal(result.Body.Bytes(), &conversation); err != nil || conversation.Data.ID == "" {
			t.Fatalf("cannot create conversation: %s", result.Body.String())
		}
		return conversation.Data.ID
	}
	selectedBotID := "openai-default"
	sendChat := func(conversationID, text, imageURL string) map[string]any {
		form := url.Values{"conversation_id": {conversationID}, "bot_id": {selectedBotID}, "content": {text}}
		if imageURL != "" {
			form.Set("up_type", "image")
			form.Set("url", imageURL)
		}
		result := authenticatedRequest(t, application, accessToken, "/api/agents/chat", "application/x-www-form-urlencoded", form.Encode())
		if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), `"code":0`) {
			t.Fatalf("web chat failed: %d %s", result.Code, result.Body.String())
		}
		select {
		case payload := <-upstreamRequests:
			return payload
		case <-time.After(5 * time.Second):
			t.Fatal("chat did not reach upstream")
			return nil
		}
	}
	conversationID := createConversation()
	first := sendChat(conversationID, "remember this picture", "https://image.example/remember.png")
	if len(first["messages"].([]any)) != 2 {
		t.Fatal("first turn should contain only system and current user messages")
	}
	sendChat(conversationID, "simulate-failure", "")
	cleanup()
	saved, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	application, management, cleanup, err = buildApplication(context.Background(), saved)
	if err != nil {
		t.Fatalf("cannot restart with persistent conversations: %v", err)
	}
	defer cleanup()
	second := sendChat(conversationID, "what picture did I send?", "")
	messages := second["messages"].([]any)
	encodedMessages, _ := json.Marshal(messages)
	if len(messages) != 4 || !strings.Contains(string(encodedMessages), "https://image.example/remember.png") || strings.Contains(string(encodedMessages), "simulate-failure") {
		t.Fatalf("context after restart lost the image or included a failed turn: %s", encodedMessages)
	}
	third := sendChat(conversationID, "latest turn only", "")
	encodedMessages, _ = json.Marshal(third["messages"])
	if len(third["messages"].([]any)) != 4 || strings.Contains(string(encodedMessages), "remember.png") || !strings.Contains(string(encodedMessages), "what picture did I send?") {
		t.Fatalf("configured context-turn limit did not retain whole recent turns: %s", encodedMessages)
	}
	separate := sendChat(createConversation(), "new conversation", "")
	if len(separate["messages"].([]any)) != 2 {
		t.Fatal("history leaked into a different conversation")
	}
	cleanup()
	saved.ContextTurns = 0
	application, management, cleanup, err = buildApplication(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	fullHistory := sendChat(conversationID, "all successful turns", "")
	encodedMessages, _ = json.Marshal(fullHistory["messages"])
	if len(fullHistory["messages"].([]any)) != 8 || !strings.Contains(string(encodedMessages), "remember.png") || strings.Contains(string(encodedMessages), "simulate-failure") {
		t.Fatalf("zero context-turn limit did not restore all successful turns: %s", encodedMessages)
	}
	selectedBotID = modelList.Data.Bots[1].BotID
	switched := sendChat(conversationID, "switch to the second model", "")
	if switched["model"] != "second-model" || len(switched["messages"].([]any)) != 10 {
		t.Fatal("selected model was ignored or switching lost the conversation context")
	}
	response = sendSetupRequest(t, management, requestBody, bootstrap.BootstrapToken(), "", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatal("the old bootstrap key can still change settings after restart")
	}
	requestBody["AdminPassword"] = "must-not-reset-existing-admin"
	response = sendSetupRequest(t, management, requestBody, "", "setupadmin", "verification-admin-password")
	if response.Code != http.StatusOK {
		t.Fatalf("administrator configuration update: %d %s", response.Code, response.Body.String())
	}
	if login("verification-admin-password") != 200 || login("must-not-reset-existing-admin") == 200 {
		t.Fatal("a configuration update reset an existing administrator password")
	}
}

func authenticatedRequest(t *testing.T, application http.Handler, token, path, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	application.ServeHTTP(response, request)
	return response
}

func sendSetupRequest(t *testing.T, handler http.Handler, body any, token, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://localhost/setup/api", bytes.NewReader(encoded))
	request.Header.Set("X-Setup-Request", "1")
	request.Header.Set("X-Setup-Token", token)
	if username != "" {
		request.SetBasicAuth(username, password)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
