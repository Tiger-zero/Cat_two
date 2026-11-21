package postgres

import (
	"context"
	"database/sql"

	"github.com/usememos/memos/store"
)

func (d *DB) ListMemoTrashAttachmentCleanup(ctx context.Context, afterID int32, nowTs int64, limit int) ([]*store.Attachment, error) {
	var attachments []*store.Attachment
	err := appendDeleteUserAttachments(ctx, d.db, `
        SELECT attachment_id, uid, creator_id, memo_id, storage_type, reference, payload
        FROM attachment_storage_cleanup
        WHERE attachment_id > $1 AND next_attempt_ts <= $2 ORDER BY attachment_id LIMIT $3`,
		[]any{afterID, nowTs, limit}, make(map[int32]struct{}), &attachments)
	return attachments, err
}

func (d *DB) ClaimMemoTrashAttachmentCleanup(ctx context.Context, attachmentID int32, nowTs, retryTs int64) (bool, error) {
	result, err := d.db.ExecContext(ctx, `UPDATE attachment_storage_cleanup SET next_attempt_ts = $1
        WHERE attachment_id = $2 AND next_attempt_ts <= $3`, retryTs, attachmentID, nowTs)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (d *DB) CompleteMemoTrashAttachmentCleanup(ctx context.Context, attachmentID int32, retryTs int64) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM attachment_storage_cleanup
        WHERE attachment_id = $1 AND next_attempt_ts = $2`, attachmentID, retryTs)
	return err
}

func (d *DB) ListLocalAttachmentStorageMatches(ctx context.Context, basename, suffixPattern string, replacingID int32, includePending bool) (*sql.Rows, error) {
	const where = ` WHERE storage_type = 'LOCAL' AND (reference = $1 OR reference LIKE $2 ESCAPE '!')`
	query := `SELECT reference FROM attachment` + where + ` AND id <> $3`
	args := []any{basename, suffixPattern, replacingID}
	if includePending {
		query += ` UNION ALL SELECT reference FROM attachment_storage_cleanup` + where
	}
	return d.db.QueryContext(ctx, query, args...)
}

func (d *DB) ListS3AttachmentStorageMatches(ctx context.Context, key string) ([]*store.Attachment, error) {
	var attachments []*store.Attachment
	err := appendDeleteUserAttachments(ctx, d.db, `
        SELECT id, uid, creator_id, memo_id, storage_type, reference, payload
        FROM attachment WHERE storage_type = 'S3' AND COALESCE(payload::json->'s3Object'->>'key', payload::json->'s3_object'->>'key') = $1`,
		[]any{key}, make(map[int32]struct{}), &attachments)
	return attachments, err
}
