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
