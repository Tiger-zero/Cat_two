ALTER TABLE memo ADD COLUMN deleted_ts INTEGER;
ALTER TABLE memo ADD COLUMN trash_root_id INTEGER;

CREATE INDEX idx_memo_trash_expiry ON memo(deleted_ts, id);
CREATE INDEX idx_memo_trash_creator ON memo(creator_id, deleted_ts, id);
CREATE INDEX idx_memo_trash_root ON memo(trash_root_id);
