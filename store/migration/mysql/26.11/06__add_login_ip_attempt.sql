CREATE TABLE IF NOT EXISTS login_ip_attempt (
  ip VARCHAR(45) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
  failure_count INTEGER NOT NULL,
  blocked_ts BIGINT
) ENGINE=InnoDB;
