CREATE TABLE IF NOT EXISTS login_ip_attempt (
  ip TEXT NOT NULL PRIMARY KEY,
  failure_count INTEGER NOT NULL CHECK (failure_count > 0),
  blocked_ts BIGINT
);

CREATE INDEX IF NOT EXISTS idx_login_ip_attempt_blocked_ip ON login_ip_attempt (ip) WHERE blocked_ts IS NOT NULL;
