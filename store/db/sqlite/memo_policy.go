package sqlite

import (
	"context"
	"database/sql"
	stderrors "errors"

	"github.com/usememos/memos/store"
)

// readSQLiteMemoActor reads the actor's lifecycle state and instance role
// inside the mutation transaction. A missing user is the zero state.
func readSQLiteMemoActor(ctx context.Context, executor dbExecutor, userID int32) (store.MemoActorState, error) {
	var rowStatus store.RowStatus
	var role store.Role
	err := executor.QueryRowContext(ctx, "SELECT row_status, role FROM user WHERE id = ?", userID).Scan(&rowStatus, &role)
	if stderrors.Is(err, sql.ErrNoRows) {
		return store.MemoActorState{}, nil
	}
	if err != nil {
		return store.MemoActorState{}, err
	}
	return store.NewMemoActorState(rowStatus, role), nil
}

// requireSQLiteActiveMemoActor resolves an actor that must be active;
// anything else is a permission denial.
func requireSQLiteActiveMemoActor(ctx context.Context, executor dbExecutor, userID int32) (store.MemoActorState, error) {
	actor, err := readSQLiteMemoActor(ctx, executor, userID)
	if err != nil {
		return store.MemoActorState{}, err
	}
	if !actor.Active {
		return store.MemoActorState{}, store.ErrMemoPermissionDenied
	}
	return actor, nil
}

// validateSQLiteMemoWritePolicy authorizes a transport-facing memo mutation
// against current database state and returns the validated snapshot.
func validateSQLiteMemoWritePolicy(ctx context.Context, executor dbExecutor, memoID int32, policy *store.MemoWritePolicy, update *store.UpdateMemo) (*store.MemoWriteSnapshot, error) {
	actor, err := requireSQLiteActiveMemoActor(ctx, executor, policy.ActorUserID)
	if err != nil {
		return nil, err
	}
	snapshot := &store.MemoWriteSnapshot{ActorIsAdmin: actor.Admin}
	if err := executor.QueryRowContext(ctx, `SELECT creator_id, visibility FROM memo WHERE id = ? AND deleted_ts IS NULL`, memoID).Scan(
		&snapshot.CreatorID, &snapshot.Visibility,
	); err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrMemoMutationConflict
		}
		return nil, err
	}
	return snapshot, store.ValidateMemoWriteSnapshot(policy, update, snapshot)
}
