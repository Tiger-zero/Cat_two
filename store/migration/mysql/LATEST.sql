-- system_setting
CREATE TABLE `system_setting` (
  `name` VARCHAR(256) NOT NULL PRIMARY KEY,
  `value` LONGTEXT NOT NULL,
  `description` TEXT NOT NULL
);

-- user
CREATE TABLE `user` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `created_ts` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_ts` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `row_status` VARCHAR(256) NOT NULL DEFAULT 'NORMAL',
  `username` VARCHAR(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL UNIQUE,
  `role` VARCHAR(256) NOT NULL DEFAULT 'USER',
  `email` VARCHAR(256) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL,
  `nickname` VARCHAR(256) NOT NULL DEFAULT '',
  `password_hash` VARCHAR(256) NOT NULL,
  `avatar_url` LONGTEXT NOT NULL,
  `description` VARCHAR(256) NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX `idx_user_email` ON `user` (`email`);

-- user_totp
CREATE TABLE user_totp (
  user_id INTEGER NOT NULL PRIMARY KEY,
  active_secret VARCHAR(255) NOT NULL,
  pending_secret VARCHAR(255) NOT NULL,
  pending_expires_ts BIGINT NOT NULL,
  last_used_step BIGINT NOT NULL,
  session_revision VARCHAR(64) NOT NULL,
  failed_attempts INTEGER NOT NULL,
  attempt_window_ts BIGINT NOT NULL,
  FOREIGN KEY (user_id) REFERENCES `user`(id) ON DELETE CASCADE
);

-- mcp_connection
CREATE TABLE mcp_connection (
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

-- user_setting
CREATE TABLE `user_setting` (
  `user_id` INT NOT NULL,
  `key` VARCHAR(256) NOT NULL,
  `value` LONGTEXT NOT NULL,
  UNIQUE(`user_id`,`key`)
);

-- space
CREATE TABLE `space` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `uid` VARCHAR(256) NOT NULL UNIQUE,
  `title` TEXT NOT NULL,
  `description` TEXT NOT NULL,
  `payload` JSON NOT NULL
);

-- space membership
CREATE TABLE `space_member` (
  `space_id` INT NOT NULL,
  `user_id` INT NOT NULL,
  `status` VARCHAR(256) NOT NULL,
  `role` VARCHAR(256) NOT NULL CHECK (`role` IN ('ADMIN', 'USER')),
  PRIMARY KEY (`space_id`, `user_id`)
);

CREATE INDEX `idx_space_member_user_id` ON `space_member`(`user_id`, `space_id`);

-- memo
CREATE TABLE `memo` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `uid` VARCHAR(256) NOT NULL UNIQUE,
  `creator_id` INT NOT NULL,
  `created_ts` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_ts` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `row_status` VARCHAR(256) NOT NULL DEFAULT 'NORMAL',
  `content` TEXT NOT NULL,
  deleted_ts BIGINT,
  trash_root_id INTEGER,
  `visibility` VARCHAR(256) NOT NULL DEFAULT 'PRIVATE',
  `pinned` BOOLEAN NOT NULL DEFAULT FALSE,
  `payload` JSON NOT NULL,
  `space_id` INT DEFAULT NULL
);

CREATE INDEX `idx_memo_space_id` ON `memo`(`space_id`, `row_status`, `created_ts`, `id`);
CREATE INDEX idx_memo_trash_expiry ON memo(deleted_ts, id);
CREATE INDEX idx_memo_trash_creator ON memo(creator_id, deleted_ts, id);
CREATE INDEX idx_memo_trash_root ON memo(trash_root_id);
CREATE INDEX `idx_memo_creator_id` ON `memo`(`creator_id`);

-- memo_relation
CREATE TABLE `memo_relation` (
  `memo_id` INT NOT NULL,
  `related_memo_id` INT NOT NULL,
  `type` VARCHAR(256) NOT NULL,
  UNIQUE(`memo_id`,`related_memo_id`,`type`)
);

CREATE INDEX `idx_memo_relation_related_type_memo`
  ON `memo_relation`(`related_memo_id`, `type`, `memo_id`);

-- attachment
CREATE TABLE `attachment` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `uid` VARCHAR(256) NOT NULL UNIQUE,
  `creator_id` INT NOT NULL,
  `created_ts` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_ts` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `filename` TEXT NOT NULL,
  `blob` MEDIUMBLOB,
  `type` VARCHAR(256) NOT NULL DEFAULT '',
  `size` INT NOT NULL DEFAULT '0',
  `memo_id` INT DEFAULT NULL,
  `storage_type` VARCHAR(256) NOT NULL DEFAULT '',
  `reference` TEXT NOT NULL DEFAULT (''),
  `payload` TEXT NOT NULL
);

CREATE INDEX `idx_attachment_creator_id` ON `attachment`(`creator_id`);
CREATE INDEX `idx_attachment_memo_id_type` ON `attachment`(`memo_id`, `type`);

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

-- idp
CREATE TABLE `idp` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `uid` VARCHAR(256) NOT NULL UNIQUE,
  `name` TEXT NOT NULL,
  `type` TEXT NOT NULL,
  `identifier_filter` VARCHAR(256) NOT NULL DEFAULT '',
  `config` TEXT NOT NULL
);

-- inbox
CREATE TABLE `inbox` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `created_ts` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `sender_id` INT NOT NULL,
  `receiver_id` INT NOT NULL,
  `status` TEXT NOT NULL,
  `message` TEXT NOT NULL
);

-- memo reaction
CREATE TABLE `reaction` (
  `id` INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `created_ts` TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `creator_id` INT NOT NULL,
  `memo_id` INT NOT NULL,
  `reaction_type` VARCHAR(256) NOT NULL,
  UNIQUE(`creator_id`,`memo_id`,`reaction_type`)
);

-- memo_share
CREATE TABLE `memo_share` (
  `id`         INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `uid`        VARCHAR(255) NOT NULL UNIQUE,
  `memo_id`    INT          NOT NULL,
  `creator_id` INT          NOT NULL,
  `created_ts` BIGINT       NOT NULL DEFAULT (UNIX_TIMESTAMP()),
  `expires_ts` BIGINT       DEFAULT NULL,
  FOREIGN KEY (`memo_id`) REFERENCES `memo`(`id`) ON DELETE CASCADE
);

CREATE INDEX `idx_memo_share_memo_id` ON `memo_share`(`memo_id`);

-- user_identity
CREATE TABLE `user_identity` (
  `id`         INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
  `user_id`    INT          NOT NULL,
  `provider`   VARCHAR(256) NOT NULL,
  `extern_uid` VARCHAR(256) NOT NULL,
  `created_ts` BIGINT       NOT NULL DEFAULT (UNIX_TIMESTAMP()),
  `updated_ts` BIGINT       NOT NULL DEFAULT (UNIX_TIMESTAMP()),
  UNIQUE (`provider`, `extern_uid`),
  UNIQUE (`user_id`, `provider`)
);

CREATE INDEX `idx_user_identity_user_id` ON `user_identity`(`user_id`);
