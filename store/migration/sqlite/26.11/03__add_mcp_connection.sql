CREATE TABLE IF NOT EXISTS mcp_connection (
  id TEXT NOT NULL PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE CASCADE,
  label TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL CHECK (status IN ('awaiting', 'pending', 'approved', 'denied', 'revoked')),
  created_ts BIGINT NOT NULL,
  requested_ts BIGINT NOT NULL DEFAULT 0,
  expires_ts BIGINT NOT NULL DEFAULT 0,
  remote_ip TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_mcp_connection_user_created ON mcp_connection (user_id, created_ts, id);
