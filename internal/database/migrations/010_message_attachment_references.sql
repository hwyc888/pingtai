-- Persist safe attachment reference metadata without storing uploaded file bytes.
ALTER TABLE ai_message ADD COLUMN attachments_json TEXT NOT NULL DEFAULT '[]';
