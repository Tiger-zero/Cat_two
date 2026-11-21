CREATE TABLE IF NOT EXISTS mcp_connection (
  id VARBINARY(64) NOT NULL PRIMARY KEY,
  user_id INTEGER NOT NULL,
  label TEXT CHARACTER SET utf8mb4 NOT NULL,
  token_hash VARBINARY(64) NOT NULL UNIQUE,
  status VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL CHECK (status IN ('awaiting', 'pending', 'approved', 'denied', 'revoked')),
  created_ts BIGINT NOT NULL,
  requested_ts BIGINT NOT NULL DEFAULT 0,
  expires_ts BIGINT NOT NULL DEFAULT 0,
  remote_ip VARCHAR(64) NOT NULL DEFAULT '',
  INDEX idx_mcp_connection_user_created (user_id, created_ts, id),
  FOREIGN KEY (user_id) REFERENCES `user`(id) ON DELETE CASCADE
) ENGINE=InnoDB;
