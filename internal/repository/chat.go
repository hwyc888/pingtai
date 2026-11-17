package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
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

// SaveMessageAttachments stores references only. The request file contents
// remain in the model context, not in a separate downloadable attachment store.
func (r *ChatRepository) SaveMessageAttachments(ctx context.Context, messageID int64, attachments []model.MessageAttachment) error {
	encoded, err := json.Marshal(attachments)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "UPDATE ai_message SET attachments_json=? WHERE id=? AND role='user'", string(encoded), messageID)
	return err
}

func scanMessageWithAttachments(row interface{ Scan(...any) error }) (model.Message, error) {
	var item model.Message
	var encoded string
	err := row.Scan(&item.ID, &item.ConversationID, &item.ChatID, &item.Role, &item.Content,
		&item.ReasoningContent, &item.CreatedAt, &encoded)
	if err != nil {
		return model.Message{}, err
	}
	if err := json.Unmarshal([]byte(encoded), &item.Attachments); err != nil {
		return model.Message{}, err
	}
	return item, nil
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

// ContextMessages selects completed turns and intentionally retained stopped turns, never failed requests.
func (r *ChatRepository) ContextMessages(ctx context.Context, conversationID string, turns int) ([]map[string]any, error) {
	// Message IDs provide deterministic ordering even when multiple turns share a DATETIME second.
	completedChats := "SELECT c.id FROM ai_chat c INNER JOIN ai_message latest ON latest.chat_id=c.id " +
		"WHERE c.conversation_id=? AND c.status IN ('completed','stopped') GROUP BY c.id ORDER BY MAX(latest.id) DESC"
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
	query := "SELECT id,conversation_id,chat_id,role,content,COALESCE(reasoning_content,''),created_at,COALESCE(attachments_json,'[]') " +
		"FROM ai_message WHERE conversation_id=? ORDER BY id " + direction + " LIMIT ?"
	rows, err := r.db.QueryContext(ctx, query, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Message, 0)
	for rows.Next() {
		item, err := scanMessageWithAttachments(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *ChatRepository) MessagesByChat(ctx context.Context, chatID, conversationID, userID string) ([]model.Message, error) {
	query := "SELECT m.id,m.conversation_id,m.chat_id,m.role,m.content,COALESCE(m.reasoning_content,''),m.created_at,COALESCE(m.attachments_json,'[]') " +
		"FROM ai_message m INNER JOIN ai_conversation v ON v.id=m.conversation_id " +
		"WHERE m.chat_id=? AND m.conversation_id=? AND v.user_id=? ORDER BY m.id ASC"
	rows, err := r.db.QueryContext(ctx, query, chatID, conversationID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Message, 0)
	for rows.Next() {
		item, err := scanMessageWithAttachments(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *ChatRepository) FindMessage(ctx context.Context, id int64, conversationID, userID string) (model.Message, error) {
	query := "SELECT m.id,m.conversation_id,m.chat_id,m.role,m.content,COALESCE(m.reasoning_content,''),m.created_at,COALESCE(m.attachments_json,'[]') " +
		"FROM ai_message m INNER JOIN ai_conversation v ON v.id=m.conversation_id " +
		"WHERE m.id=? AND m.conversation_id=? AND v.user_id=? LIMIT 1"
	return scanMessageWithAttachments(r.db.QueryRowContext(ctx, query, id, conversationID, userID))
}


type ConversationSummary struct {
	ID        string    `json:"id"`
	BotID     string    `json:"bot_id"`
	Title     string    `json:"title"`
	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
}

func (r *ChatRepository) ListConversations(ctx context.Context, userID string, limit int) ([]ConversationSummary, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	query := `SELECT v.id,v.bot_id,v.created_at,v.updated_at,
		COALESCE((SELECT CASE WHEN m.role='user' THEN m.content ELSE '' END
			FROM ai_message m WHERE m.conversation_id=v.id AND m.role='user'
			ORDER BY m.id ASC LIMIT 1),'')
		FROM ai_conversation v
		WHERE v.user_id=? AND EXISTS (SELECT 1 FROM ai_message m WHERE m.conversation_id=v.id)
		ORDER BY v.updated_at DESC,v.created_at DESC LIMIT ?`
	rows, err := r.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ConversationSummary, 0)
	for rows.Next() {
		var item ConversationSummary
		if err := rows.Scan(&item.ID, &item.BotID, &item.CreatedAt, &item.UpdatedAt, &item.Title); err != nil {
			return nil, err
		}
		item.Title = strings.TrimSpace(item.Title)
		if item.Title == "" {
			item.Title = "新对话"
		}
		runes := []rune(item.Title)
		if len(runes) > 60 {
			item.Title = string(runes[:60]) + "…"
		}
		items = append(items, item)
	}
	return items, rows.Err()
}


func (r *ChatRepository) ClearConversation(ctx context.Context, id, userID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM ai_conversation WHERE id=? AND user_id=?", id, userID).Scan(&exists); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM ai_message_context WHERE message_id IN (SELECT id FROM ai_message WHERE conversation_id=?)",
		id,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM ai_message WHERE conversation_id=?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM ai_chat WHERE conversation_id=?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE ai_conversation SET updated_at=? WHERE id=? AND user_id=?",
		time.Now(), id, userID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *ChatRepository) ClearAllConversations(ctx context.Context, userID string) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		"DELETE FROM ai_message_context WHERE message_id IN ("+
			"SELECT m.id FROM ai_message m INNER JOIN ai_conversation v ON v.id=m.conversation_id WHERE v.user_id=?)",
		userID,
	); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM ai_message WHERE conversation_id IN (SELECT id FROM ai_conversation WHERE user_id=?)",
		userID,
	); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM ai_chat WHERE conversation_id IN (SELECT id FROM ai_conversation WHERE user_id=?)",
		userID,
	); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM ai_conversation WHERE user_id=?", userID)
	if err != nil {
		return 0, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (r *ChatRepository) DeleteConversation(ctx context.Context, id, userID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM ai_conversation WHERE id=? AND user_id=?", id, userID).Scan(&exists); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM ai_message_context WHERE message_id IN (SELECT id FROM ai_message WHERE conversation_id=?)", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM ai_message WHERE conversation_id=?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM ai_chat WHERE conversation_id=?", id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM ai_conversation WHERE id=? AND user_id=?", id, userID); err != nil {
		return err
	}
	return tx.Commit()
}

 // PrepareMessageEdit removes the selected user turn and every later turn in
 // the same conversation. The client then resubmits the edited text as a new turn.
func (r *ChatRepository) PrepareMessageEdit(ctx context.Context, messageID int64, conversationID, userID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var role, status string
	err = tx.QueryRowContext(ctx,
		"SELECT m.role,COALESCE(c.status,'') FROM ai_message m "+
			"INNER JOIN ai_conversation v ON v.id=m.conversation_id "+
			"LEFT JOIN ai_chat c ON c.id=m.chat_id "+
			"WHERE m.id=? AND m.conversation_id=? AND v.user_id=? LIMIT 1",
		messageID, conversationID, userID,
	).Scan(&role, &status)
	if err != nil {
		return err
	}
	if role != "user" {
		return errors.New("only user messages can be edited")
	}
	if status == "in_progress" {
		return errors.New("this message is still generating a response")
	}

	if _, err := tx.ExecContext(ctx,
		"DELETE FROM ai_message_context WHERE message_id IN "+
			"(SELECT id FROM ai_message WHERE conversation_id=? AND id>=?)",
		conversationID, messageID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM ai_chat WHERE conversation_id=? AND id IN "+
			"(SELECT DISTINCT chat_id FROM ai_message WHERE conversation_id=? AND id>=?)",
		conversationID, conversationID, messageID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"DELETE FROM ai_message WHERE conversation_id=? AND id>=?",
		conversationID, messageID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE ai_conversation SET updated_at=? WHERE id=? AND user_id=?",
		time.Now(), conversationID, userID,
	); err != nil {
		return err
	}
	return tx.Commit()
}
