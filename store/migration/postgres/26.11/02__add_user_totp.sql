CREATE TABLE IF NOT EXISTS user_totp (
  user_id INTEGER NOT NULL PRIMARY KEY REFERENCES "user"(id) ON DELETE CASCADE,
  active_secret VARCHAR(255) NOT NULL,
  pending_secret VARCHAR(255) NOT NULL,
  pending_expires_ts BIGINT NOT NULL,
  last_used_step BIGINT NOT NULL,
  session_revision VARCHAR(64) NOT NULL,
  failed_attempts INTEGER NOT NULL,
  attempt_window_ts BIGINT NOT NULL
);
