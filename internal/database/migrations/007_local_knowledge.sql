CREATE TABLE IF NOT EXISTS ai_knowledge_base (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_base_enabled
  ON ai_knowledge_base(enabled, updated_at);

CREATE TABLE IF NOT EXISTS ai_knowledge_document (
  id TEXT PRIMARY KEY,
  base_id TEXT NOT NULL,
  filename TEXT NOT NULL,
  content_type TEXT NOT NULL DEFAULT '',
  size_bytes INTEGER NOT NULL DEFAULT 0,
  content_text TEXT NOT NULL,
  chunk_count INTEGER NOT NULL DEFAULT 0,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(base_id) REFERENCES ai_knowledge_base(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_document_base
  ON ai_knowledge_document(base_id, created_at DESC);

CREATE TABLE IF NOT EXISTS ai_knowledge_chunk (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  base_id TEXT NOT NULL,
  document_id TEXT NOT NULL,
  chunk_index INTEGER NOT NULL,
  content TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(base_id) REFERENCES ai_knowledge_base(id) ON DELETE CASCADE,
  FOREIGN KEY(document_id) REFERENCES ai_knowledge_document(id) ON DELETE CASCADE,
  UNIQUE(document_id, chunk_index)
);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_chunk_base
  ON ai_knowledge_chunk(base_id, document_id);
