CREATE TABLE IF NOT EXISTS ai_provider (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  api_key TEXT NOT NULL DEFAULT '',
  protocol TEXT NOT NULL DEFAULT 'openai',
  active INTEGER NOT NULL DEFAULT 0,
  default_model TEXT NOT NULL DEFAULT '',
  max_output_tokens INTEGER NOT NULL DEFAULT 0,
  timeout_seconds INTEGER NOT NULL DEFAULT 180,
  system_prompt TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_provider_name ON ai_provider(name);
CREATE INDEX IF NOT EXISTS idx_ai_provider_active ON ai_provider(active);

CREATE TABLE IF NOT EXISTS ai_provider_model (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  provider_id INTEGER NOT NULL,
  model_id TEXT NOT NULL,
  context_length INTEGER,
  max_output_tokens INTEGER,
  enabled INTEGER NOT NULL DEFAULT 1,
  discovered_at DATETIME,
  FOREIGN KEY(provider_id) REFERENCES ai_provider(id) ON DELETE CASCADE,
  UNIQUE(provider_id, model_id)
);
CREATE INDEX IF NOT EXISTS idx_ai_provider_model_provider ON ai_provider_model(provider_id, enabled, model_id);
