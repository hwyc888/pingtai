CREATE TABLE IF NOT EXISTS ai_platform_setting (
  setting_key TEXT PRIMARY KEY,
  setting_value TEXT NOT NULL,
  updated_at DATETIME NOT NULL
);
