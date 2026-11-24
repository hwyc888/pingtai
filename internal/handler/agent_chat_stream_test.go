package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pingtai/internal/database"
	"pingtai/internal/model"
	"pingtai/internal/repository"
)

type streamResponseClient struct {
	response *http.Response
	err      error
}

func (client streamResponseClient) Do(context.Context, []byte) (*http.Response, error) {
	return client.response, client.err
}

type blockingStreamBody struct {
	ctx      context.Context
	sent     bool
	first    chan struct{}
}

func (body *blockingStreamBody) Read(buffer []byte) (int, error) {
	if !body.sent {
		body.sent = true
		data := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial answer\"}}]}\n\n")
		n := copy(buffer, data)
		close(body.first)
		return n, nil
	}
	<-body.ctx.Done()
	return 0, body.ctx.Err()
}

func (body *blockingStreamBody) Close() error { return nil }

func newStreamTestRepository(t *testing.T) (*repository.ChatRepository, func()) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, db); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	repo := repository.NewChatRepository(db)
	if err := repo.CreateConversation(ctx, "conversation-stream", "alice", "bot"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := repo.CreateChat(ctx, "chat-stream", "conversation-stream", "in_progress"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-stream", "chat-stream", "user", "question", ""); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return repo, func() { _ = db.Close() }
}

func TestStreamModelChatPersistsAndStreamsCompletedAnswer(t *testing.T) {
	repo, closeDB := newStreamTestRepository(t)
	defer closeDB()
	upstream := "data: {\"choices\":[{\"delta\":{\"content\":\"hello \"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"world\"}}]}\n\n" +
		"data: [DONE]\n\n"
	handler := &Handler{Chats: repo}
	request := httptest.NewRequest(http.MethodPost, "/agents/chat", nil)
	recorder := httptest.NewRecorder()
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(upstream))}

	sources := []model.MessageKnowledgeSource{{
		FileID: "file-stream", DocumentID: "doc-stream", BaseID: "base-stream",
		BaseName: "学校规章", FileName: "教学制度.pdf", Snippet: "教学制度来源片段",
	}}
	handler.streamModelChatWithSources(recorder, request, streamResponseClient{response: response}, "chat-stream", "conversation-stream", 1, nil, sources)

	if !strings.Contains(recorder.Body.String(), "event: delta") || !strings.Contains(recorder.Body.String(), "event: done") ||
		!strings.Contains(recorder.Body.String(), "file-stream") {
		t.Fatalf("expected SSE deltas, completion and knowledge source metadata, got %s", recorder.Body.String())
	}
	chat, err := repo.FindChat(context.Background(), "chat-stream", "conversation-stream", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if chat.Status != "completed" {
		t.Fatalf("expected completed status, got %q", chat.Status)
	}
	messages, err := repo.MessagesByChat(context.Background(), "chat-stream", "conversation-stream", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].Content != "hello world" ||
		len(messages[1].KnowledgeSources) != 1 || messages[1].KnowledgeSources[0].FileID != "file-stream" {
		t.Fatalf("unexpected persisted messages or knowledge sources: %#v", messages)
	}
}

func TestStreamModelChatKeepsPartialAnswerWhenCancelled(t *testing.T) {
	repo, closeDB := newStreamTestRepository(t)
	defer closeDB()
	ctx, cancel := context.WithCancel(context.Background())
	body := &blockingStreamBody{ctx: ctx, first: make(chan struct{})}
	response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: body}
	handler := &Handler{Chats: repo}
	request := httptest.NewRequest(http.MethodPost, "/agents/chat", nil).WithContext(ctx)
	recorder := httptest.NewRecorder()
	done := make(chan struct{})

	go func() {
		handler.streamModelChat(recorder, request, streamResponseClient{response: response}, "chat-stream", "conversation-stream", 1, nil)
		close(done)
	}()
	<-body.first
	cancel()
	<-done

	chat, err := repo.FindChat(context.Background(), "chat-stream", "conversation-stream", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if chat.Status != "stopped" {
		t.Fatalf("expected stopped status, got %q", chat.Status)
	}
	messages, err := repo.MessagesByChat(context.Background(), "chat-stream", "conversation-stream", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[1].Content != "partial answer" {
		t.Fatalf("partial assistant answer was not saved: %#v", messages)
	}
}
