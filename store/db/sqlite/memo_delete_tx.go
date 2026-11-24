package sqlite

import (
	"context"

	"github.com/pkg/errors"

	"github.com/usememos/memos/store"
)

func deleteSQLiteMemoSetTx(ctx context.Context, tx dbExecutor, memoIDs []int32) ([]*store.Attachment, error) {
	attachments, err := listSQLiteMemoSetAttachments(ctx, tx, memoIDs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect memo attachments")
	}
	if err := deleteSQLiteMemoSetShares(ctx, tx, memoIDs); err != nil {
		return nil, errors.Wrap(err, "failed to delete memo shares")
	}
	if err := deleteMemosTx(ctx, tx, memoIDs); err != nil {
		return nil, errors.Wrap(err, "failed to delete memos")
	}
	if err := deleteReactionsByMemoIDsTx(ctx, tx, memoIDs); err != nil {
		return nil, errors.Wrap(err, "failed to delete memo reactions")
	}
	if err := deleteAttachmentsByIDsTx(ctx, tx, attachmentIDsFromList(attachments)); err != nil {
		return nil, errors.Wrap(err, "failed to delete memo attachments")
	}
	if err := deleteMemoRelationsTx(ctx, tx, memoIDs); err != nil {
		return nil, errors.Wrap(err, "failed to delete incident memo relations")
	}
	return attachments, nil
}

func listSQLiteMemoSetAttachments(ctx context.Context, tx dbExecutor, memoIDs []int32) ([]*store.Attachment, error) {
	attachments := make([]*store.Attachment, 0)
	seen := make(map[int32]struct{})
	for _, batch := range deleteUserBatches(memoIDs, deleteUserBatchSize) {
		clause, args := deleteUserInClause(1, batch)
		if err := appendDeleteUserAttachments(ctx, tx, `SELECT id, uid, creator_id, memo_id, storage_type, reference, payload
			FROM attachment WHERE memo_id IN `+clause+` ORDER BY id`, args, seen, &attachments); err != nil {
			return nil, err
		}
	}
	return attachments, nil
}

func deleteSQLiteMemoSetShares(ctx context.Context, tx dbExecutor, memoIDs []int32) error {
	for _, batch := range deleteUserBatches(memoIDs, deleteUserBatchSize) {
		clause, args := deleteUserInClause(1, batch)
		if _, err := tx.ExecContext(ctx, "DELETE FROM memo_share WHERE memo_id IN "+clause, args...); err != nil {
			return err
		}
	}
	return nil
}
