-- system_setting
CREATE TABLE system_setting (
  name TEXT NOT NULL,
  value TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  UNIQUE(name)
);

-- user
CREATE TABLE user (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now')),
  updated_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now')),
  row_status TEXT NOT NULL CHECK (row_status IN ('NORMAL', 'ARCHIVED')) DEFAULT 'NORMAL',
  username TEXT COLLATE BINARY NOT NULL UNIQUE,
  role TEXT NOT NULL DEFAULT 'USER',
  email TEXT COLLATE BINARY DEFAULT NULL,
  nickname TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL,
  avatar_url TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX idx_user_email ON user(email);

-- user_totp
CREATE TABLE user_totp (
  user_id INTEGER NOT NULL PRIMARY KEY REFERENCES user(id) ON DELETE CASCADE,
  active_secret VARCHAR(255) NOT NULL,
  pending_secret VARCHAR(255) NOT NULL,
  pending_expires_ts BIGINT NOT NULL,
  last_used_step BIGINT NOT NULL,
  session_revision VARCHAR(64) NOT NULL,
  failed_attempts INTEGER NOT NULL,
  attempt_window_ts BIGINT NOT NULL
);

-- mcp_connection
CREATE TABLE mcp_connection (
  id TEXT NOT NULL PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE CASCADE,
  label TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL CHECK (status IN ('approved', 'revoked')),
  created_ts BIGINT NOT NULL,
  requested_ts BIGINT NOT NULL DEFAULT 0,
  expires_ts BIGINT NOT NULL DEFAULT 0,
  remote_ip TEXT NOT NULL DEFAULT '',
  permission TEXT NOT NULL DEFAULT 'read_write' CHECK (permission IN ('read_only', 'read_write'))
);

CREATE INDEX idx_mcp_connection_user_created ON mcp_connection (user_id, created_ts, id);

-- login_ip_attempt
CREATE TABLE login_ip_attempt (
  ip TEXT NOT NULL PRIMARY KEY,
  failure_count INTEGER NOT NULL CHECK (failure_count > 0),
  blocked_ts BIGINT
);

CREATE INDEX idx_login_ip_attempt_blocked_ip ON login_ip_attempt (ip) WHERE blocked_ts IS NOT NULL;

-- user_setting
CREATE TABLE user_setting (
  user_id INTEGER NOT NULL,
  key TEXT NOT NULL,
  value TEXT NOT NULL,
  UNIQUE(user_id, key)
);

-- memo
CREATE TABLE memo (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  uid TEXT NOT NULL UNIQUE,
  creator_id INTEGER NOT NULL,
  created_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now')),
  updated_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now')),
  content TEXT NOT NULL DEFAULT '',
  body_digest TEXT NOT NULL DEFAULT '',
  revision BIGINT NOT NULL DEFAULT 1,
  creation_request_digest TEXT NOT NULL DEFAULT '',
  deleted_ts INTEGER,
  trash_root_id INTEGER,
  visibility TEXT NOT NULL CHECK (visibility IN ('PUBLIC', 'PROTECTED', 'PRIVATE')) DEFAULT 'PRIVATE',
  pinned INTEGER NOT NULL CHECK (pinned IN (0, 1)) DEFAULT 0,
  payload TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_memo_creator_id ON memo(creator_id);
CREATE INDEX idx_memo_trash_expiry ON memo(deleted_ts, id);
CREATE INDEX idx_memo_trash_creator ON memo(creator_id, deleted_ts, id);
CREATE INDEX idx_memo_trash_root ON memo(trash_root_id);
CREATE INDEX idx_memo_body_digest ON memo(body_digest) WHERE body_digest != '';
CREATE INDEX idx_memo_owner_pinned_created
  ON memo(creator_id, pinned DESC, created_ts DESC, id DESC)
  WHERE deleted_ts IS NULL;

-- memo_body_namespace
CREATE TABLE memo_body_namespace (
  id INTEGER NOT NULL PRIMARY KEY CHECK (id = 1),
  namespace TEXT NOT NULL
);

INSERT INTO memo_body_namespace (id, namespace) VALUES (1, lower(hex(randomblob(16))));

-- memo_relation
CREATE TABLE memo_relation (
  memo_id INTEGER NOT NULL,
  related_memo_id INTEGER NOT NULL,
  type TEXT NOT NULL,
  UNIQUE(memo_id, related_memo_id, type)
);

CREATE INDEX idx_memo_relation_related_type_memo
  ON memo_relation(related_memo_id, type, memo_id);

-- attachment
CREATE TABLE attachment (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  uid TEXT NOT NULL UNIQUE,
  creator_id INTEGER NOT NULL,
  created_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now')),
  updated_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now')),
  filename TEXT NOT NULL DEFAULT '',
  blob BLOB DEFAULT NULL,
  type TEXT NOT NULL DEFAULT '',
  size INTEGER NOT NULL DEFAULT 0,
  memo_id INTEGER,
  storage_type TEXT NOT NULL DEFAULT '',
  reference TEXT NOT NULL DEFAULT '',
  payload TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_attachment_creator_id ON attachment(creator_id);
CREATE INDEX idx_attachment_memo_id_type ON attachment(memo_id, type);

-- Removed attachment metadata for retrying trash storage cleanup after commit.
CREATE TABLE attachment_storage_cleanup (
  attachment_id INTEGER PRIMARY KEY,
  uid TEXT NOT NULL,
  creator_id INTEGER NOT NULL,
  memo_id INTEGER,
  storage_type TEXT NOT NULL,
  reference TEXT NOT NULL,
  payload TEXT NOT NULL,
  next_attempt_ts BIGINT NOT NULL DEFAULT 0
);

-- inbox
CREATE TABLE inbox (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now')),
  sender_id INTEGER NOT NULL,
  receiver_id INTEGER NOT NULL,
  status TEXT NOT NULL,
  message TEXT NOT NULL DEFAULT '{}'
);

-- memo reaction
CREATE TABLE reaction (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_ts BIGINT NOT NULL DEFAULT (strftime('%s', 'now')),
  creator_id INTEGER NOT NULL,
  memo_id INTEGER NOT NULL,
  reaction_type TEXT NOT NULL,
  UNIQUE(creator_id, memo_id, reaction_type)
);

-- memo_share
CREATE TABLE memo_share (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  uid        TEXT    NOT NULL UNIQUE,
  memo_id    INTEGER NOT NULL,
  creator_id INTEGER NOT NULL,
  created_ts BIGINT  NOT NULL DEFAULT (strftime('%s', 'now')),
  expires_ts BIGINT  DEFAULT NULL,
  FOREIGN KEY (memo_id) REFERENCES memo(id) ON DELETE CASCADE
);

CREATE INDEX idx_memo_share_memo_id ON memo_share(memo_id);
