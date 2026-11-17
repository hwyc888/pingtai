package model

import "time"

type Conversation struct {
	ID        string    "json:\"id\""
	UserID    string    "json:\"user_id\""
	BotID     string    "json:\"bot_id\""
	CreatedAt time.Time "json:\"created_at\""
	UpdatedAt time.Time "json:\"updated_at\""
}

type Chat struct {
	ID             string    "json:\"id\""
	ConversationID string    "json:\"conversation_id\""
	Status         string    "json:\"status\""
	ErrorMessage   string    "json:\"error_message,omitempty\""
	CreatedAt      time.Time "json:\"created_at\""
	UpdatedAt      time.Time "json:\"updated_at\""
}

type MessageAttachment struct {
	Name string `json:"name"`
	Size int64 `json:"size"`
	Type string `json:"type"`
}

type Message struct {
	ID               int64     "json:\"id\""
	ConversationID   string    "json:\"conversation_id\""
	ChatID           string    "json:\"chat_id\""
	Role             string    "json:\"role\""
	Content          string    "json:\"content\""
	ReasoningContent string    "json:\"reasoning_content,omitempty\""
	Attachments      []MessageAttachment `json:"attachments,omitempty"`
	CreatedAt        time.Time "json:\"created_at\""
}
