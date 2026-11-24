CREATE TABLE IF NOT EXISTS ai_knowledge_document_file (
  document_id TEXT NOT NULL PRIMARY KEY,
  base_id TEXT NOT NULL,
  file_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  FOREIGN KEY(document_id) REFERENCES ai_knowledge_document(id) ON DELETE CASCADE,
  FOREIGN KEY(base_id) REFERENCES ai_knowledge_base(id) ON DELETE CASCADE,
  FOREIGN KEY(file_id) REFERENCES platform_file(id) ON DELETE RESTRICT,
  UNIQUE(base_id, file_id)
);

CREATE INDEX IF NOT EXISTS idx_knowledge_document_file_file
  ON ai_knowledge_document_file(file_id, base_id);

CREATE INDEX IF NOT EXISTS idx_knowledge_document_file_base
  ON ai_knowledge_document_file(base_id, created_at DESC);
