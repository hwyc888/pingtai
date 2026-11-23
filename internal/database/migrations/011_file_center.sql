CREATE TABLE IF NOT EXISTS platform_file (
  id TEXT PRIMARY KEY,
  storage_key TEXT NOT NULL UNIQUE,
  original_name TEXT NOT NULL,
  content_type TEXT NOT NULL DEFAULT '',
  extension TEXT NOT NULL DEFAULT '',
  size_bytes INTEGER NOT NULL DEFAULT 0,
  sha256 TEXT NOT NULL,
  extracted_text TEXT NOT NULL DEFAULT '',
  extraction_status TEXT NOT NULL DEFAULT 'filename_only',
  extraction_error TEXT NOT NULL DEFAULT '',
  owner_user_id TEXT NOT NULL,
  owner_username TEXT NOT NULL,
  owner_provider TEXT NOT NULL DEFAULT 'local',
  access_mode TEXT NOT NULL DEFAULT 'private' CHECK (access_mode IN ('private','shared')),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(owner_user_id, sha256)
);

CREATE INDEX IF NOT EXISTS idx_platform_file_owner
  ON platform_file(owner_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_platform_file_shared
  ON platform_file(access_mode, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_platform_file_extension
  ON platform_file(extension, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_platform_file_name
  ON platform_file(original_name COLLATE NOCASE);
