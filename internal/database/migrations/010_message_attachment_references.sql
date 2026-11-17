-- File reference metadata in an idempotent table
-- Uploaded file bytes stay in the existing model context
CREATE TABLE IF NOT EXISTS ai_message_attachment (
  message_id INTEGER NOT NULL PRIMARY KEY,
  attachments_json TEXT NOT NULL DEFAULT '[]',
  FOREIGN KEY(message_id) REFERENCES ai_message(id) ON DELETE CASCADE
);
