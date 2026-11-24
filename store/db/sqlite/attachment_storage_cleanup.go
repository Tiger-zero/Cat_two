package sqlite

import (
	"context"
	"database/sql"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

const enqueueAttachmentStorageCleanupSQL = `
        INSERT INTO attachment_storage_cleanup (attachment_id, uid, creator_id, memo_id, storage_type, reference, payload)
        VALUES (?, ?, ?, ?, ?, ?, ?)`

func (d *DB) ListMemoTrashAttachmentCleanup(ctx context.Context, afterID int32, nowTs int64, limit int) ([]*store.Attachment, error) {
	var attachments []*store.Attachment
	err := appendDeleteUserAttachments(ctx, d.db, `
        SELECT attachment_id, uid, creator_id, memo_id, storage_type, reference, '{}'
        FROM attachment_storage_cleanup
        WHERE attachment_id > ? AND next_attempt_ts <= ? ORDER BY attachment_id LIMIT ?`,
		[]any{afterID, nowTs, limit}, make(map[int32]struct{}), &attachments)
	return attachments, err
}

func (d *DB) ClaimMemoTrashAttachmentCleanup(ctx context.Context, attachmentID int32, nowTs, retryTs int64) (bool, error) {
	result, err := d.db.ExecContext(ctx, `UPDATE attachment_storage_cleanup SET next_attempt_ts = ?
        WHERE attachment_id = ? AND next_attempt_ts <= ?`, retryTs, attachmentID, nowTs)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (d *DB) CompleteMemoTrashAttachmentCleanup(ctx context.Context, attachmentID int32, retryTs int64) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM attachment_storage_cleanup
        WHERE attachment_id = ? AND next_attempt_ts = ?`, attachmentID, retryTs)
	return err
}

func (d *DB) ListLocalAttachmentStorageMatches(ctx context.Context, basename, suffixPattern string, replacingID int32, includePending bool) (*sql.Rows, error) {
	const where = ` WHERE storage_type = 'LOCAL' AND (reference = ? OR reference LIKE ? ESCAPE '!')`
	query := `SELECT reference FROM attachment` + where + ` AND id <> ?`
	args := []any{basename, suffixPattern, replacingID}
	if includePending {
		query += ` UNION ALL SELECT reference FROM attachment_storage_cleanup` + where
		args = append(args, basename, suffixPattern)
	}
	return d.db.QueryContext(ctx, query, args...)
}

func appendDeleteUserAttachments(ctx context.Context, tx dbExecutor, query string, args []any, seen map[int32]struct{}, attachments *[]*store.Attachment) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		attachment := &store.Attachment{}
		var memoID sql.NullInt32
		var storageType string
		var payloadBytes []byte
		if err := rows.Scan(&attachment.ID, &attachment.UID, &attachment.CreatorID, &memoID, &storageType, &attachment.Reference, &payloadBytes); err != nil {
			return err
		}
		if _, exists := seen[attachment.ID]; exists {
			continue
		}
		seen[attachment.ID] = struct{}{}
		if memoID.Valid {
			attachment.MemoID = &memoID.Int32
		}
		attachment.StorageType = storepb.AttachmentStorageType(storepb.AttachmentStorageType_value[storageType])
		payload := &storepb.AttachmentPayload{}
		if len(payloadBytes) > 0 {
			if err := protojsonUnmarshaler.Unmarshal(payloadBytes, payload); err != nil {
				return err
			}
		}
		attachment.Payload = payload
		*attachments = append(*attachments, attachment)
	}
	return rows.Err()
}
