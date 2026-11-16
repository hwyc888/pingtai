package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pingtai/internal/config"
	"pingtai/internal/database"
	"pingtai/internal/middleware"
	"pingtai/internal/repository"
	authservice "pingtai/internal/service/auth"
)

func TestKnowledgeChunkManagementViewEditAndAdd(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}

	users := repository.NewUserRepository(db)
	for _, username := range []string{"alice", "bob"} {
		if _, err := users.Create(ctx, username, "test-password", 0); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.ExecContext(ctx,
		"INSERT INTO ai_knowledge_base (id,name,description,enabled,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?)",
		"kb-chunks", "测试知识库", "", 1, "alice", now, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO ai_knowledge_base_policy (base_id,owner_username,access_mode) VALUES (?,?,?)",
		"kb-chunks", "alice", "assigned",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO ai_knowledge_base_member (base_id,username) VALUES (?,?)",
		"kb-chunks", "bob",
	); err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(ctx,
		"INSERT INTO ai_knowledge_document (id,base_id,filename,content_type,size_bytes,content_text,chunk_count,created_by,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)",
		"doc-chunks", "kb-chunks", "manual.txt", "text/plain", 20, "original text", 1, "alice", now, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO ai_knowledge_chunk (base_id,document_id,chunk_index,content,created_at) VALUES (?,?,?,?,?)",
		"kb-chunks", "doc-chunks", 0, "original chunk content", now,
	); err != nil {
		t.Fatal(err)
	}

	handler := &Handler{
		Config: &config.Config{},
		DB: db,
		Users: users,
	}
	tokens := authservice.NewTokenManager("knowledge-chunk-test-secret", time.Hour, 24*time.Hour)
	call := func(username, method, path, body string, endpoint http.HandlerFunc) *httptest.ResponseRecorder {
		t.Helper()
		token, err := tokens.IssueAccess(username)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		middleware.RequireAccess(tokens, endpoint).ServeHTTP(recorder, request)
		return recorder
	}

	list := call("alice", http.MethodGet, "/knowledge/manage/chunks?document_id=doc-chunks", "",
		handler.ListLocalKnowledgeChunks)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "original chunk content") {
		t.Fatalf("owner should be able to view chunks: status=%d body=%s", list.Code, list.Body.String())
	}

	memberUpdate := call("bob", http.MethodPost, "/knowledge/manage/chunks/update",
		`{"chunk_id":1,"content":"unauthorized change"}`, handler.UpdateLocalKnowledgeChunk)
	if memberUpdate.Code != http.StatusForbidden {
		t.Fatalf("an assigned search-only member must not edit chunks: status=%d body=%s", memberUpdate.Code, memberUpdate.Body.String())
	}

	update := call("alice", http.MethodPost, "/knowledge/manage/chunks/update",
		`{"chunk_id":1,"content":"updated needle content"}`, handler.UpdateLocalKnowledgeChunk)
	if update.Code != http.StatusOK {
		t.Fatalf("owner should be able to update a chunk: status=%d body=%s", update.Code, update.Body.String())
	}
	var updatedContent string
	if err := db.QueryRowContext(ctx, "SELECT content FROM ai_knowledge_chunk WHERE id=1").Scan(&updatedContent); err != nil {
		t.Fatal(err)
	}
	if updatedContent != "updated needle content" {
		t.Fatalf("chunk content not updated: %q", updatedContent)
	}
	prompt, err := handler.knowledgePrompt(ctx, "alice", "kb-chunks", "needle")
	if err != nil || !strings.Contains(prompt, "updated needle content") {
		t.Fatalf("edited chunk should be immediately searchable: prompt=%q err=%v", prompt, err)
	}

	create := call("alice", http.MethodPost, "/knowledge/manage/chunks/create",
		`{"document_id":"doc-chunks","content":"manually added supplemental material"}`, handler.CreateLocalKnowledgeChunk)
	if create.Code != http.StatusOK {
		t.Fatalf("owner should be able to add a chunk: status=%d body=%s", create.Code, create.Body.String())
	}
	var chunkCount int
	if err := db.QueryRowContext(ctx, "SELECT chunk_count FROM ai_knowledge_document WHERE id='doc-chunks'").Scan(&chunkCount); err != nil {
		t.Fatal(err)
	}
	if chunkCount != 2 {
		t.Fatalf("document chunk_count should be updated, got %d", chunkCount)
	}
	prompt, err = handler.knowledgePrompt(ctx, "alice", "kb-chunks", "supplemental")
	if err != nil || !strings.Contains(prompt, "manually added supplemental material") {
		t.Fatalf("new chunk should be immediately searchable: prompt=%q err=%v", prompt, err)
	}

	var response map[string]any
	if err := json.Unmarshal(create.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["code"] != float64(0) {
		t.Fatalf("unexpected create response: %#v", response)
	}
}

func TestValidateKnowledgeChunkContent(t *testing.T) {
	if content, message := validateKnowledgeChunkContent("  content  "); content != "content" || message != "" {
		t.Fatalf("expected trimmed content, got %q and %q", content, message)
	}
	if _, message := validateKnowledgeChunkContent("  "); message == "" {
		t.Fatal("empty chunk content should be rejected")
	}
	if _, message := validateKnowledgeChunkContent(strings.Repeat("界", maxKnowledgeChunkRunes+1)); message == "" {
		t.Fatal("oversized chunk content should be rejected")
	}
}
