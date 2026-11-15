CREATE TABLE IF NOT EXISTS ai_conversation (
  id TEXT NOT NULL PRIMARY KEY,
  user_id TEXT NOT NULL,
  bot_id TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ai_conversation_user ON ai_conversation(user_id, updated_at);

CREATE TABLE IF NOT EXISTS ai_chat (
  id TEXT NOT NULL PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  status TEXT NOT NULL,
  error_message TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ai_chat_conversation ON ai_chat(conversation_id, created_at);

CREATE TABLE IF NOT EXISTS ai_message (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  conversation_id TEXT NOT NULL,
  chat_id TEXT NOT NULL DEFAULT '',
  role TEXT NOT NULL,
  content TEXT NOT NULL,
  reasoning_content TEXT,
  created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ai_message_conversation ON ai_message(conversation_id, id);
CREATE INDEX IF NOT EXISTS idx_ai_message_chat ON ai_message(chat_id, id);
