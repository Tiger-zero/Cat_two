package sqlite

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"

	"github.com/usememos/memos/store"
)

// A separately deleted memo, or a surviving reply whose original root was
// hard-deleted with its account/Space, can be restored or purged independently.
const memoTrashRootPredicate = `(memo.trash_root_id = memo.id OR NOT EXISTS (
    SELECT 1 FROM memo AS trash_root
    WHERE trash_root.id = memo.trash_root_id AND trash_root.deleted_ts IS NOT NULL
))`

func (d *DB) ApplyMemoTrash(ctx context.Context, mutation *store.MemoTrashMutation) ([]*store.Attachment, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to begin memo recycle-bin transaction")
	}
	defer func() { _ = tx.Rollback() }()

	if mutation.ActorUserID != 0 {
		if _, err := requireSQLiteActiveMemoActor(ctx, tx, mutation.ActorUserID); err != nil {
			return nil, err
		}
	}

	var creatorID int32
	var deletedTs *int64
	var isTrashRoot bool
	err = tx.QueryRowContext(ctx, `SELECT creator_id, deleted_ts, `+memoTrashRootPredicate+` FROM memo WHERE id = ?`, mutation.MemoID).Scan(
		&creatorID, &deletedTs, &isTrashRoot,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrMemoMutationConflict
	}
	if err != nil {
		return nil, errors.Wrap(err, "failed to read memo recycle-bin state")
	}
	if err := store.ValidateMemoTrashState(mutation, creatorID, deletedTs, isTrashRoot); err != nil {
		return nil, err
	}

	if mutation.Action == store.MemoTrashRestore {
		var parentTrashed bool
		err := tx.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM memo_relation AS relation
			JOIN memo AS parent ON parent.id = relation.related_memo_id
			WHERE relation.memo_id = ? AND relation.type = 'COMMENT' AND parent.deleted_ts IS NOT NULL
		)`, mutation.MemoID).Scan(&parentTrashed)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read parent recycle-bin state")
		}
		if parentTrashed {
			return nil, store.ErrMemoMutationConflict
		}
		if _, err := tx.ExecContext(ctx, `UPDATE memo SET deleted_ts = NULL, trash_root_id = NULL WHERE trash_root_id = ? OR id = ?`, mutation.MemoID, mutation.MemoID); err != nil {
			return nil, errors.Wrap(err, "failed to restore memo")
		}
		return nil, errors.Wrap(tx.Commit(), "failed to commit memo restore")
	}

	ids, err := listSQLiteCommentSubtreeMemoIDs(ctx, tx, mutation.MemoID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect comment memos")
	}
	if mutation.Action == store.MemoTrashMove {
		for _, batch := range deleteUserBatches(ids, deleteUserBatchSize) {
			clause, ids := deleteUserInClause(3, batch)
			args := append([]any{mutation.NowTs, mutation.MemoID}, ids...)
			if _, err := tx.ExecContext(ctx, `UPDATE memo SET deleted_ts = ?, trash_root_id = ? WHERE deleted_ts IS NULL AND id IN `+clause, args...); err != nil {
				return nil, errors.Wrap(err, "failed to move memo to the recycle bin")
			}
		}
		return nil, errors.Wrap(tx.Commit(), "failed to commit memo trash")
	}

	attachments, err := deleteSQLiteMemoSetTx(ctx, tx, ids)
	if err != nil {
		return nil, errors.Wrap(err, "failed to permanently delete memo")
	}
	if err := store.EnqueueMemoTrashAttachmentCleanup(ctx, tx, attachments, `
        INSERT INTO attachment_storage_cleanup (attachment_id, uid, creator_id, memo_id, storage_type, reference, payload)
        VALUES (?, ?, ?, ?, ?, ?, ?)`); err != nil {
		return nil, errors.Wrap(err, "failed to queue recycle-bin attachment storage cleanup")
	}
	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, "failed to commit permanent memo deletion")
	}
	return attachments, nil
}
