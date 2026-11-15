-- SQLite schema for pingtai Go runtime.
-- The application applies the versioned, idempotent migrations in internal/database/migrations.
-- This file is provided for inspection/manual initialization only.

CREATE TABLE IF NOT EXISTS ai_user (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password TEXT NOT NULL,
  usertype INTEGER NOT NULL DEFAULT 0,
  status INTEGER NOT NULL DEFAULT 1,
  deltag INTEGER NOT NULL DEFAULT 0,
  openid TEXT DEFAULT NULL,
  bind_time DATETIME DEFAULT NULL,
  created_at DATETIME DEFAULT NULL,
  updated_at DATETIME DEFAULT NULL
);

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

CREATE TABLE IF NOT EXISTS ai_message_context (
  message_id INTEGER NOT NULL PRIMARY KEY,
  content_json TEXT NOT NULL
);
