CREATE TABLE IF NOT EXISTS ai_message_context (
  message_id BIGINT NOT NULL,
  content_json LONGTEXT NOT NULL,
  PRIMARY KEY (message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
