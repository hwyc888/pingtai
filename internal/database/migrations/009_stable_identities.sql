-- Stable identities separate the account's display name from its data ownership.
CREATE TABLE IF NOT EXISTS ai_identity (
  id TEXT NOT NULL PRIMARY KEY,
  provider TEXT NOT NULL CHECK(provider IN ('local','sso')),
  subject TEXT NOT NULL,
  username TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  local_user_id INTEGER,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(provider, subject),
  UNIQUE(local_user_id),
  FOREIGN KEY(local_user_id) REFERENCES ai_user(id)
);
CREATE INDEX IF NOT EXISTS idx_ai_identity_username ON ai_identity(username, provider);

-- Local identities are stable and directly tied to the existing integer user record.
INSERT OR IGNORE INTO ai_identity (id,provider,subject,username,display_name,local_user_id,created_at,updated_at)
SELECT 'local-' || id,'local',username,username,username,id,
       COALESCE(created_at,CURRENT_TIMESTAMP),COALESCE(updated_at,CURRENT_TIMESTAMP)
FROM ai_user;

-- Older SSO-only conversations and knowledge permissions used the account name as the owner.
-- Their original provider cannot be inferred if a local account with the same name already existed,
-- in that ambiguous legacy case the local identity is preferred. New sessions are provider-scoped.
INSERT OR IGNORE INTO ai_identity (id,provider,subject,username,display_name,created_at,updated_at)
SELECT 'sso-' || lower(hex(randomblob(16))),'sso',legacy.username,legacy.username,legacy.username,
       CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
FROM (
  SELECT user_id AS username FROM ai_conversation
  UNION
  SELECT owner_username AS username FROM ai_knowledge_base_policy
  UNION
  SELECT username FROM ai_knowledge_base_member WHERE username NOT LIKE 'sso:%'
  UNION
  SELECT created_by AS username FROM ai_knowledge_base
) AS legacy
WHERE trim(COALESCE(legacy.username,'')) <> ''
  AND NOT EXISTS (SELECT 1 FROM ai_identity i WHERE i.username=legacy.username);

-- Migrate existing local conversation ownership from usernames to stable identity IDs.
UPDATE ai_conversation
SET user_id = (
  SELECT i.id FROM ai_identity i
  WHERE i.username=ai_conversation.user_id
  ORDER BY CASE WHEN i.provider='local' THEN 0 ELSE 1 END
  LIMIT 1
)
WHERE EXISTS (SELECT 1 FROM ai_identity i WHERE i.username=ai_conversation.user_id);

-- Knowledge base ownership and membership are now identity based, legacy username tables
-- remain as display/backward-compatibility data only.
CREATE TABLE IF NOT EXISTS ai_knowledge_base_identity (
  base_id TEXT NOT NULL PRIMARY KEY,
  owner_user_id TEXT NOT NULL,
  access_mode TEXT NOT NULL CHECK(access_mode IN ('shared','assigned','private')),
  FOREIGN KEY(base_id) REFERENCES ai_knowledge_base(id) ON DELETE CASCADE,
  FOREIGN KEY(owner_user_id) REFERENCES ai_identity(id)
);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_identity_owner
  ON ai_knowledge_base_identity(owner_user_id, access_mode);

CREATE TABLE IF NOT EXISTS ai_knowledge_base_identity_member (
  base_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  PRIMARY KEY(base_id,user_id),
  FOREIGN KEY(base_id) REFERENCES ai_knowledge_base(id) ON DELETE CASCADE,
  FOREIGN KEY(user_id) REFERENCES ai_identity(id)
);
CREATE INDEX IF NOT EXISTS idx_ai_knowledge_identity_member
  ON ai_knowledge_base_identity_member(user_id,base_id);

INSERT OR IGNORE INTO ai_knowledge_base_identity (base_id,owner_user_id,access_mode)
SELECT p.base_id,
       (SELECT i.id FROM ai_identity i WHERE i.username=p.owner_username
        ORDER BY CASE WHEN i.provider='local' THEN 0 ELSE 1 END LIMIT 1),
       p.access_mode
FROM ai_knowledge_base_policy p
WHERE EXISTS (SELECT 1 FROM ai_identity i WHERE i.username=p.owner_username);

INSERT OR IGNORE INTO ai_knowledge_base_identity_member (base_id,user_id)
SELECT m.base_id,
       (SELECT i.id FROM ai_identity i WHERE i.username=m.username
        ORDER BY CASE WHEN i.provider='local' THEN 0 ELSE 1 END LIMIT 1)
FROM ai_knowledge_base_member m
WHERE m.username NOT LIKE 'sso:%'
  AND EXISTS (SELECT 1 FROM ai_identity i WHERE i.username=m.username);
