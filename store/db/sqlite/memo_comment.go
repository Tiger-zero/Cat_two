package sqlite

import (
	"context"
	"database/sql"
	stderrors "errors"

	"github.com/usememos/memos/store"
)

func authorizeSQLiteMemoComment(ctx context.Context, tx dbExecutor, mutation *store.MemoMutation) error {
	snapshot, err := loadSQLiteMemoParticipation(ctx, tx, *mutation.CommentContextMemoID, mutation.MemoCreate.CreatorID)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return store.ErrMemoSpaceNotWritable
		}
		return err
	}
	return mutation.AuthorizeCommentContext(snapshot)
}

// loadSQLiteMemoParticipation reads the actor and note in the write transaction.
func loadSQLiteMemoParticipation(ctx context.Context, tx dbExecutor, memoID, actorUserID int32) (*store.MemoCommentAuthorizationSnapshot, error) {
	actor, err := readSQLiteMemoActor(ctx, tx, actorUserID)
	if err != nil {
		return nil, err
	}
	snapshot := &store.MemoCommentAuthorizationSnapshot{ActorUserID: actorUserID, Actor: actor, ContextID: memoID}
	if err := tx.QueryRowContext(ctx, `SELECT creator_id, row_status, visibility FROM memo WHERE id = ? AND deleted_ts IS NULL`, memoID).Scan(
		&snapshot.ContextCreatorID, &snapshot.ContextRowStatus, &snapshot.ContextVisibility,
	); err != nil {
		return nil, err
	}
	return snapshot, nil
}
