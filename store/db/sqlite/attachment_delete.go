package sqlite

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"

	"github.com/usememos/memos/store"
)

func (d *DB) DeleteAttachmentsWithPolicy(ctx context.Context, policy *store.AttachmentDeletionPolicy, attachmentIDs []int32) error {
	if policy == nil || policy.ActorUserID <= 0 {
		return store.ErrMemoPermissionDenied
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "failed to begin attachment delete transaction")
	}
	defer func() { _ = tx.Rollback() }()
	actor, err := requireSQLiteActiveMemoActor(ctx, tx, policy.ActorUserID)
	if err != nil {
		return err
	}
	attachments, err := listSQLiteAttachmentSnapshots(ctx, tx, attachmentIDs)
	if err != nil {
		return errors.Wrap(err, "failed to read attachment delete targets")
	}
	memoIDs, err := store.ValidateAttachmentMutationTargets(policy.ActorUserID, actor.Admin, attachmentIDs, attachments)
	if err != nil {
		return err
	}
	if err := store.ValidateAttachmentDeletionMemoSnapshots(memoIDs, policy.ExpectedMemoContents); err != nil {
		return err
	}
	if err := authorizeSQLiteAttachmentMutation(ctx, tx, policy.ActorUserID, actor.Admin, memoIDs, policy.ExpectedMemoContents); err != nil {
		return err
	}

	for _, attachment := range attachments {
		result, err := tx.ExecContext(ctx, "DELETE FROM attachment WHERE id = ? AND memo_id IS ?", attachment.ID, attachment.MemoID)
		if err != nil {
			return errors.Wrap(err, "failed to delete attachment")
		}
		if rows, err := result.RowsAffected(); err != nil {
			return errors.Wrap(err, "failed to count deleted attachment")
		} else if rows != 1 {
			return store.ErrMemoMutationConflict
		}
	}
	for _, memoID := range memoIDs {
		if err := bumpMemoRevision(ctx, tx, memoID); err != nil {
			return err
		}
	}
	if err := store.EnqueueAttachmentStorageCleanup(ctx, tx, attachments, enqueueAttachmentStorageCleanupSQL); err != nil {
		return errors.Wrap(err, "failed to queue deleted attachment storage cleanup")
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "failed to commit attachment deletion")
	}
	return nil
}

func listSQLiteAttachmentSnapshots(ctx context.Context, tx dbExecutor, attachmentIDs []int32) ([]*store.Attachment, error) {
	attachments := make([]*store.Attachment, 0, len(attachmentIDs))
	seen := make(map[int32]struct{}, len(attachmentIDs))
	for _, batch := range deleteUserBatches(attachmentIDs, deleteUserBatchSize) {
		clause, args := deleteUserInClause(1, batch)
		if err := appendDeleteUserAttachments(ctx, tx, `SELECT id, uid, creator_id, memo_id, storage_type, reference, payload
			FROM attachment WHERE id IN `+clause+` ORDER BY id`, args, seen, &attachments); err != nil {
			return nil, err
		}
	}
	return attachments, nil
}

func authorizeSQLiteAttachmentMutation(
	ctx context.Context,
	tx dbExecutor,
	actorUserID int32,
	actorIsAdmin bool,
	memoIDs []int32,
	expectedMemoContents map[int32]string,
) error {
	for _, memoID := range memoIDs {
		snapshot := &store.MemoWriteSnapshot{ActorIsAdmin: actorIsAdmin}
		var content string
		if err := tx.QueryRowContext(ctx, `SELECT creator_id, row_status, visibility, content
			FROM memo WHERE id = ? AND deleted_ts IS NULL`, memoID).Scan(
			&snapshot.CreatorID, &snapshot.RowStatus, &snapshot.Visibility, &content,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return store.ErrMemoMutationConflict
			}
			return errors.Wrap(err, "failed to read attachment memo")
		}
		if expectedMemoContents != nil && content != expectedMemoContents[memoID] {
			return store.ErrMemoMutationConflict
		}
		if err := store.ValidateMemoWriteSnapshot(&store.MemoWritePolicy{ActorUserID: actorUserID}, nil, snapshot); err != nil {
			return err
		}
	}
	return nil
}
