package repository

import (
	"context"
	"testing"

	"pingtai/internal/database"
)

func TestConversationHistoryListsOnlyUsersOwnNonEmptyConversations(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := NewChatRepository(db)
	if err := repo.CreateConversation(ctx, "conversation-a", "alice", "bot"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateConversation(ctx, "conversation-b", "alice", "bot"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateConversation(ctx, "conversation-c", "bob", "bot"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateChat(ctx, "chat-a", "conversation-a", "in_progress"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-a", "chat-a", "user", "first question", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateChat(ctx, "chat-a", "completed", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateChat(ctx, "chat-c", "conversation-c", "in_progress"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-c", "chat-c", "user", "bob secret", ""); err != nil {
		t.Fatal(err)
	}
	items, err := repo.ListConversations(ctx, "alice", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "conversation-a" || items[0].Title != "first question" {
		t.Fatalf("unexpected history items: %#v", items)
	}
}

func TestDeleteConversationRemovesOnlyOwnedConversationAndMessages(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := NewChatRepository(db)
	if err := repo.CreateConversation(ctx, "conversation-a", "alice", "bot"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateChat(ctx, "chat-a", "conversation-a", "completed"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-a", "chat-a", "user", "question", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteConversation(ctx, "conversation-a", "bob"); err == nil {
		t.Fatal("another user must not delete this conversation")
	}
	if err := repo.DeleteConversation(ctx, "conversation-a", "alice"); err != nil {
		t.Fatal(err)
	}
	var conversations, chats, messages int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_conversation").Scan(&conversations); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_chat").Scan(&chats); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_message").Scan(&messages); err != nil {
		t.Fatal(err)
	}
	if conversations != 0 || chats != 0 || messages != 0 {
		t.Fatalf("conversation cleanup incomplete: conversations=%d chats=%d messages=%d", conversations, chats, messages)
	}
}

func TestContextMessagesIncludesStoppedPartialAnswers(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := NewChatRepository(db)
	if err := repo.CreateConversation(ctx, "conversation-stopped", "alice", "bot"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateChat(ctx, "chat-stopped", "conversation-stopped", "in_progress"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-stopped", "chat-stopped", "user", "question", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-stopped", "chat-stopped", "assistant", "partial answer", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateChat(ctx, "chat-stopped", "stopped", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateChat(ctx, "chat-failed", "conversation-stopped", "in_progress"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-stopped", "chat-failed", "user", "failed question", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-stopped", "chat-failed", "assistant", "must not enter context", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateChat(ctx, "chat-failed", "failed", "upstream error"); err != nil {
		t.Fatal(err)
	}

	messages, err := repo.ContextMessages(ctx, "conversation-stopped", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected the stopped turn to remain in context, got %#v", messages)
	}
	if messages[0]["content"] != "question" || messages[1]["content"] != "partial answer" {
		t.Fatalf("unexpected stopped turn context: %#v", messages)
	}
}
