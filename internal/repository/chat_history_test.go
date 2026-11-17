package repository

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"pingtai/internal/database"
	"pingtai/internal/model"
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

func TestPrepareMessageEditRemovesSelectedAndLaterTurns(t *testing.T) {
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
	if err := repo.CreateConversation(ctx, "conversation-edit", "alice", "bot"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateChat(ctx, "chat-first", "conversation-edit", "completed"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-edit", "chat-first", "user", "first question", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-edit", "chat-first", "assistant", "first answer", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateChat(ctx, "chat-second", "conversation-edit", "completed"); err != nil {
		t.Fatal(err)
	}
	edited, err := repo.InsertMessage(ctx, "conversation-edit", "chat-second", "user", "old question", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-edit", "chat-second", "assistant", "old answer", ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateChat(ctx, "chat-third", "conversation-edit", "completed"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-edit", "chat-third", "user", "later question", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-edit", "chat-third", "assistant", "later answer", ""); err != nil {
		t.Fatal(err)
	}

	if err := repo.PrepareMessageEdit(ctx, edited.ID, "conversation-edit", "alice"); err != nil {
		t.Fatal(err)
	}
	messages, err := repo.ListMessages(ctx, "conversation-edit", 100, "asc")
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Content != "first question" || messages[1].Content != "first answer" {
		t.Fatalf("edit should retain only earlier turns: %#v", messages)
	}
	var chats int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_chat WHERE conversation_id=?", "conversation-edit").Scan(&chats); err != nil {
		t.Fatal(err)
	}
	if chats != 1 {
		t.Fatalf("expected later chat rows removed, got %d", chats)
	}
}

func TestPrepareMessageEditRequiresOwnedUserMessageAndCompletedTurn(t *testing.T) {
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
	if err := repo.CreateConversation(ctx, "conversation-edit", "alice", "bot"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateChat(ctx, "chat-edit", "conversation-edit", "in_progress"); err != nil {
		t.Fatal(err)
	}
	userMessage, err := repo.InsertMessage(ctx, "conversation-edit", "chat-edit", "user", "question", "")
	if err != nil {
		t.Fatal(err)
	}
	assistantMessage, err := repo.InsertMessage(ctx, "conversation-edit", "chat-edit", "assistant", "answer", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.PrepareMessageEdit(ctx, userMessage.ID, "conversation-edit", "bob"); err == nil {
		t.Fatal("another user must not edit this message")
	}
	if err := repo.PrepareMessageEdit(ctx, assistantMessage.ID, "conversation-edit", "alice"); err == nil {
		t.Fatal("assistant messages must not be editable")
	}
	if err := repo.PrepareMessageEdit(ctx, userMessage.ID, "conversation-edit", "alice"); err == nil {
		t.Fatal("an in-progress turn must not be editable")
	}
}

func TestClearConversationKeepsConversationButRemovesMessages(t *testing.T) {
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
	if err := repo.CreateConversation(ctx, "conversation-clear", "alice", "bot"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateChat(ctx, "chat-clear", "conversation-clear", "completed"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-clear", "chat-clear", "user", "question", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-clear", "chat-clear", "assistant", "answer", ""); err != nil {
		t.Fatal(err)
	}

	if err := repo.ClearConversation(ctx, "conversation-clear", "bob"); err == nil {
		t.Fatal("another user must not clear this conversation")
	}
	if err := repo.ClearConversation(ctx, "conversation-clear", "alice"); err != nil {
		t.Fatal(err)
	}

	var conversations, chats, messages int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_conversation WHERE id=?", "conversation-clear").Scan(&conversations); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_chat WHERE conversation_id=?", "conversation-clear").Scan(&chats); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_message WHERE conversation_id=?", "conversation-clear").Scan(&messages); err != nil {
		t.Fatal(err)
	}
	if conversations != 1 || chats != 0 || messages != 0 {
		t.Fatalf("clear current conversation result: conversations=%d chats=%d messages=%d", conversations, chats, messages)
	}
}

func TestClearAllConversationsOnlyDeletesCurrentUsersHistory(t *testing.T) {
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
	for _, item := range []struct {
		id    string
		owner string
		chat  string
		text  string
	}{
		{id: "alice-a", owner: "alice", chat: "chat-alice-a", text: "alice question"},
		{id: "alice-b", owner: "alice", chat: "chat-alice-b", text: "alice second question"},
		{id: "bob-a", owner: "bob", chat: "chat-bob-a", text: "bob private question"},
	} {
		if err := repo.CreateConversation(ctx, item.id, item.owner, "bot"); err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateChat(ctx, item.chat, item.id, "completed"); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.InsertMessage(ctx, item.id, item.chat, "user", item.text, ""); err != nil {
			t.Fatal(err)
		}
	}
	deleted, err := repo.ClearAllConversations(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Fatalf("expected two conversations deleted, got %d", deleted)
	}

	var aliceConversations, bobConversations, aliceChats, bobChats, aliceMessages, bobMessages int
	checks := []struct {
		query string
		args  []any
		dest  *int
	}{
		{"SELECT COUNT(*) FROM ai_conversation WHERE user_id='alice'", nil, &aliceConversations},
		{"SELECT COUNT(*) FROM ai_conversation WHERE user_id='bob'", nil, &bobConversations},
		{"SELECT COUNT(*) FROM ai_chat WHERE conversation_id IN (SELECT id FROM ai_conversation WHERE user_id='alice')", nil, &aliceChats},
		{"SELECT COUNT(*) FROM ai_chat WHERE conversation_id IN (SELECT id FROM ai_conversation WHERE user_id='bob')", nil, &bobChats},
		{"SELECT COUNT(*) FROM ai_message WHERE conversation_id IN (SELECT id FROM ai_conversation WHERE user_id='alice')", nil, &aliceMessages},
		{"SELECT COUNT(*) FROM ai_message WHERE conversation_id IN (SELECT id FROM ai_conversation WHERE user_id='bob')", nil, &bobMessages},
	}
	for _, check := range checks {
		if err := db.QueryRowContext(ctx, check.query, check.args...).Scan(check.dest); err != nil {
			t.Fatal(err)
		}
	}
	if aliceConversations != 0 || bobConversations != 1 || aliceChats != 0 || bobChats != 1 || aliceMessages != 0 || bobMessages != 1 {
		t.Fatalf("clear all history leaked across users: alice conversations/chats/messages=%d/%d/%d, bob=%d/%d/%d",
			aliceConversations, aliceChats, aliceMessages, bobConversations, bobChats, bobMessages)
	}
}


func TestChatAttachmentReferencesPersistAndContextKeepsText(t *testing.T) {
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
	if err := repo.CreateConversation(ctx, "conversation-files", "local-1", "bot"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateChat(ctx, "chat-files", "conversation-files", "completed"); err != nil {
		t.Fatal(err)
	}
	userMessage, err := repo.InsertMessage(ctx, "conversation-files", "chat-files", "user", "请总结文档", "")
	if err != nil {
		t.Fatal(err)
	}
	references := []model.MessageAttachment{
		{Name: "制度.txt", Size: 128, Type: "file"},
		{Name: "截图.png", Size: 4096, Type: "image"},
	}
	if err := repo.SaveMessageAttachments(ctx, userMessage.ID, references); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveMessageContent(ctx, userMessage.ID, "请总结文档\n\n附件 制度.txt:\n课程安排"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InsertMessage(ctx, "conversation-files", "chat-files", "assistant", "好的", ""); err != nil {
		t.Fatal(err)
	}

	items, err := repo.ListMessages(ctx, "conversation-files", 20, "asc")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Content != "请总结文档" || len(items[0].Attachments) != 2 {
		t.Fatalf("expected a text prompt and two visible file references, got %#v", items)
	}
	if items[0].Attachments[0].Name != "制度.txt" || items[0].Attachments[1].Type != "image" {
		t.Fatalf("attachment references changed: %#v", items[0].Attachments)
	}
	found, err := repo.FindMessage(ctx, userMessage.ID, "conversation-files", "local-1")
	if err != nil || len(found.Attachments) != 2 {
		t.Fatalf("message lookup lost attachments: %#v, %v", found, err)
	}
	byChat, err := repo.MessagesByChat(ctx, "chat-files", "conversation-files", "local-1")
	if err != nil || len(byChat) != 2 || len(byChat[0].Attachments) != 2 {
		t.Fatalf("chat message list lost attachments: %#v, %v", byChat, err)
	}
	contextMessages, err := repo.ContextMessages(ctx, "conversation-files", 10)
	if err != nil || len(contextMessages) != 2 {
		t.Fatalf("conversation context was not retained: %#v, %v", contextMessages, err)
	}
	if !strings.Contains(string(contextMessages[0]["content"].(json.RawMessage)), "课程安排") {
		t.Fatalf("model context lost TXT attachment contents: %#v", contextMessages[0])
	}
}
