package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"pingtai/internal/model"
)

type ChatRepository struct {
	db *sql.DB
}

func NewChatRepository(db *sql.DB) *ChatRepository {
	return &ChatRepository{db: db}
}

func (r *ChatRepository) CreateConversation(ctx context.Context, id, userID, botID string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, "INSERT INTO ai_conversation (id,user_id,bot_id,created_at,updated_at) VALUES (?,?,?,?,?)",
		id, userID, botID, now, now)
	return err
}

func (r *ChatRepository) UpdateConversationBot(ctx context.Context, id, userID, botID string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE ai_conversation SET bot_id=?,updated_at=? WHERE id=? AND user_id=?",
		botID, time.Now(), id, userID)
	return err
}

func (r *ChatRepository) FindConversation(ctx context.Context, id, userID string) (model.Conversation, error) {
	var item model.Conversation
	err := r.db.QueryRowContext(ctx, "SELECT id,user_id,bot_id,created_at,updated_at "+
		"FROM ai_conversation WHERE id=? AND user_id=? LIMIT 1", id, userID).
		Scan(&item.ID, &item.UserID, &item.BotID, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (r *ChatRepository) CreateChat(ctx context.Context, id, conversationID, status string) error {
	now := time.Now()
	_, err := r.db.ExecContext(ctx, "INSERT INTO ai_chat (id,conversation_id,status,created_at,updated_at) "+
		"VALUES (?,?,?,?,?)", id, conversationID, status, now, now)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "UPDATE ai_conversation SET updated_at=? WHERE id=?", now, conversationID)
	return err
}

func (r *ChatRepository) UpdateChat(ctx context.Context, id, status, message string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE ai_chat SET status=?,error_message=?,updated_at=? WHERE id=?",
		status, message, time.Now(), id)
	return err
}

func (r *ChatRepository) FindChat(ctx context.Context, chatID, conversationID, userID string) (model.Chat, error) {
	var item model.Chat
	query := "SELECT c.id,c.conversation_id,c.status,COALESCE(c.error_message,''),c.created_at,c.updated_at " +
		"FROM ai_chat c INNER JOIN ai_conversation v ON v.id=c.conversation_id " +
		"WHERE c.id=? AND c.conversation_id=? AND v.user_id=? LIMIT 1"
	err := r.db.QueryRowContext(ctx, query, chatID, conversationID, userID).
		Scan(&item.ID, &item.ConversationID, &item.Status, &item.ErrorMessage, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func (r *ChatRepository) InsertMessage(ctx context.Context, conversationID, chatID, role, content, reasoning string) (model.Message, error) {
	now := time.Now()
	result, err := r.db.ExecContext(ctx, "INSERT INTO ai_message "+
		"(conversation_id,chat_id,role,content,reasoning_content,created_at) VALUES (?,?,?,?,?,?)",
		conversationID, chatID, role, content, reasoning, now)
	if err != nil {
		return model.Message{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.Message{}, err
	}
	return model.Message{
		ID: id, ConversationID: conversationID, ChatID: chatID,
		Role: role, Content: content, ReasoningContent: reasoning, CreatedAt: now,
	}, nil
}

func (r *ChatRepository) LastMessages(ctx context.Context, conversationID string, limit int) ([]model.Message, error) {
	query := "SELECT id,conversation_id,chat_id,role,content,COALESCE(reasoning_content,''),created_at " +
		"FROM ai_message WHERE conversation_id=? ORDER BY id DESC LIMIT ?"
	rows, err := r.db.QueryContext(ctx, query, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Message, 0)
	for rows.Next() {
		var item model.Message
		if err := rows.Scan(&item.ID, &item.ConversationID, &item.ChatID, &item.Role, &item.Content, &item.ReasoningContent, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return items, nil
}

func (r *ChatRepository) SaveMessageContent(ctx context.Context, messageID int64, content any) error {
	encoded, err := json.Marshal(content)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "INSERT INTO ai_message_context (message_id,content_json) VALUES (?,?)", messageID, string(encoded))
	return err
}

// ContextMessages selects complete successful turns, never orphaned messages from failed requests.
func (r *ChatRepository) ContextMessages(ctx context.Context, conversationID string, turns int) ([]map[string]any, error) {
	// Message IDs provide deterministic ordering even when multiple turns share a DATETIME second.
	completedChats := "SELECT c.id FROM ai_chat c INNER JOIN ai_message latest ON latest.chat_id=c.id " +
		"WHERE c.conversation_id=? AND c.status='completed' GROUP BY c.id ORDER BY MAX(latest.id) DESC"
	arguments := []any{conversationID}
	if turns > 0 {
		completedChats += " LIMIT ?"
		arguments = append(arguments, turns)
	}
	query := "SELECT m.chat_id,m.role,m.content,p.content_json FROM ai_message m " +
		"INNER JOIN (" + completedChats + ") c ON c.id=m.chat_id " +
		"LEFT JOIN ai_message_context p ON p.message_id=m.id " +
		"WHERE m.role IN ('user','assistant') ORDER BY m.id ASC"
	rows, err := r.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]map[string]any, 0)
	for rows.Next() {
		var turnID, role, text string
		var encoded sql.NullString
		if err := rows.Scan(&turnID, &role, &text, &encoded); err != nil {
			return nil, err
		}
		var content any = text
		if encoded.Valid && json.Valid([]byte(encoded.String)) {
			content = json.RawMessage(encoded.String)
		}
		messages = append(messages, map[string]any{"turn_id": turnID, "role": role, "content": content})
	}
	return messages, rows.Err()
}

func (r *ChatRepository) ListMessages(ctx context.Context, conversationID string, limit int, order string) ([]model.Message, error) {
	direction := "DESC"
	if order == "asc" {
		direction = "ASC"
	}
	query := "SELECT id,conversation_id,chat_id,role,content,COALESCE(reasoning_content,''),created_at " +
		"FROM ai_message WHERE conversation_id=? ORDER BY id " + direction + " LIMIT ?"
	rows, err := r.db.QueryContext(ctx, query, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Message, 0)
	for rows.Next() {
		var item model.Message
		if err := rows.Scan(&item.ID, &item.ConversationID, &item.ChatID, &item.Role, &item.Content, &item.ReasoningContent, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *ChatRepository) MessagesByChat(ctx context.Context, chatID, conversationID, userID string) ([]model.Message, error) {
	query := "SELECT m.id,m.conversation_id,m.chat_id,m.role,m.content,COALESCE(m.reasoning_content,''),m.created_at " +
		"FROM ai_message m INNER JOIN ai_conversation v ON v.id=m.conversation_id " +
		"WHERE m.chat_id=? AND m.conversation_id=? AND v.user_id=? ORDER BY m.id ASC"
	rows, err := r.db.QueryContext(ctx, query, chatID, conversationID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Message, 0)
	for rows.Next() {
		var item model.Message
		if err := rows.Scan(&item.ID, &item.ConversationID, &item.ChatID, &item.Role, &item.Content, &item.ReasoningContent, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *ChatRepository) FindMessage(ctx context.Context, id int64, conversationID, userID string) (model.Message, error) {
	var item model.Message
	query := "SELECT m.id,m.conversation_id,m.chat_id,m.role,m.content,COALESCE(m.reasoning_content,''),m.created_at " +
		"FROM ai_message m INNER JOIN ai_conversation v ON v.id=m.conversation_id " +
		"WHERE m.id=? AND m.conversation_id=? AND v.user_id=? LIMIT 1"
	err := r.db.QueryRowContext(ctx, query, id, conversationID, userID).
		Scan(&item.ID, &item.ConversationID, &item.ChatID, &item.Role, &item.Content, &item.ReasoningContent, &item.CreatedAt)
	return item, err
}
