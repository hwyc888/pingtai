CREATE TABLE IF NOT EXISTS ai_conversation (
  id VARCHAR(64) NOT NULL,
  user_id VARCHAR(50) NOT NULL,
  bot_id VARCHAR(128) NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (id),
  KEY idx_ai_conversation_user (user_id, updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS ai_chat (
  id VARCHAR(64) NOT NULL,
  conversation_id VARCHAR(64) NOT NULL,
  status VARCHAR(32) NOT NULL,
  error_message TEXT,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (id),
  KEY idx_ai_chat_conversation (conversation_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS ai_message (
  id BIGINT NOT NULL AUTO_INCREMENT,
  conversation_id VARCHAR(64) NOT NULL,
  chat_id VARCHAR(64) NOT NULL DEFAULT '',
  role VARCHAR(32) NOT NULL,
  content LONGTEXT NOT NULL,
  reasoning_content LONGTEXT,
  created_at DATETIME NOT NULL,
  PRIMARY KEY (id),
  KEY idx_ai_message_conversation (conversation_id, id),
  KEY idx_ai_message_chat (chat_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
