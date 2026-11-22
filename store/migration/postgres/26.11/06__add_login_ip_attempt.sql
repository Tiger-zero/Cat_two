CREATE TABLE IF NOT EXISTS login_ip_attempt (
  ip VARCHAR(45) COLLATE "C" NOT NULL PRIMARY KEY,
  failure_count INTEGER NOT NULL,
  blocked_ts BIGINT
);

CREATE INDEX IF NOT EXISTS idx_login_ip_attempt_blocked ON login_ip_attempt (ip) WHERE blocked_ts IS NOT NULL;
