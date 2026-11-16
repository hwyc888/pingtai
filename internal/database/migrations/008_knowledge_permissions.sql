CREATE TABLE IF NOT EXISTS ai_knowledge_base_policy (
  base_id TEXT PRIMARY KEY,
  owner_username TEXT NOT NULL,
  access_mode TEXT NOT NULL CHECK(access_mode IN ('shared','assigned','private')),
  FOREIGN KEY(base_id) REFERENCES ai_knowledge_base(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_policy_owner
  ON ai_knowledge_base_policy(owner_username, access_mode);

CREATE TABLE IF NOT EXISTS ai_knowledge_base_member (
  base_id TEXT NOT NULL,
  username TEXT NOT NULL,
  PRIMARY KEY(base_id, username),
  FOREIGN KEY(base_id) REFERENCES ai_knowledge_base(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_member_username
  ON ai_knowledge_base_member(username, base_id);

INSERT OR IGNORE INTO ai_knowledge_base_policy (base_id, owner_username, access_mode)
SELECT id, created_by, 'private' FROM ai_knowledge_base;
