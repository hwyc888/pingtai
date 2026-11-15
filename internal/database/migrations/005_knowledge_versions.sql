CREATE TABLE IF NOT EXISTS ai_knowledge_version (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  dataset_id TEXT NOT NULL,
  version_number INTEGER NOT NULL,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'published',
  snapshot_json TEXT NOT NULL,
  document_count INTEGER NOT NULL DEFAULT 0,
  created_by TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  published_at TEXT NOT NULL,
  UNIQUE(dataset_id, version_number)
);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_version_dataset
  ON ai_knowledge_version(dataset_id, version_number DESC);
