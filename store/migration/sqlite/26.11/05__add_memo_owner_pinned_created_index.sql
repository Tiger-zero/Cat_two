CREATE INDEX idx_memo_owner_pinned_created
  ON memo(creator_id, row_status, pinned DESC, created_ts DESC, id DESC)
  WHERE deleted_ts IS NULL;
