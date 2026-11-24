CREATE TABLE IF NOT EXISTS ai_message_knowledge_source (
  message_id INTEGER NOT NULL PRIMARY KEY,
  sources_json TEXT NOT NULL DEFAULT '[]',
  FOREIGN KEY(message_id) REFERENCES ai_message(id) ON DELETE CASCADE
);
