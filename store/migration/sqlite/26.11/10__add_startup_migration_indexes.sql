CREATE INDEX IF NOT EXISTS idx_memo_legacy_body ON memo(id) WHERE body_digest = '';
CREATE INDEX IF NOT EXISTS idx_memo_body_digest ON memo(body_digest) WHERE body_digest != '';
CREATE INDEX IF NOT EXISTS idx_attachment_legacy_storage
  ON attachment(id) WHERE storage_type NOT IN ('LOCAL', 'EXTERNAL', 'S3');
