package sqlite

import (
	"context"
	"database/sql"
	stderrors "errors"
	"slices"

	"github.com/usememos/memos/store"
)

type sqliteRelationMemoState struct {
	id         int32
	creatorID  int32
	rowStatus  store.RowStatus
	visibility store.Visibility
}

func validateSQLiteMemoRelationEndpoints(ctx context.Context, executor dbExecutor, mutation *store.MemoMutation) error {
	if !mutation.ReplaceReferenceRelations && mutation.CommentContextMemoID == nil {
		return nil
	}

	actorUserID := mutation.ActorUserID()
	actor, err := readSQLiteMemoActor(ctx, executor, actorUserID)
	if err != nil {
		return err
	}

	endpointIDs := sqliteRelationAuthorizationEndpointIDs(mutation)
	for _, endpointID := range endpointIDs {
		state, err := getSQLiteRelationMemoState(ctx, executor, endpointID)
		if err != nil {
			if stderrors.Is(err, sql.ErrNoRows) {
				return store.ErrMemoMutationConflict
			}
			return err
		}
		snapshot := &store.MemoRelationEndpointSnapshot{
			ActorUserID:        actorUserID,
			Actor:              actor,
			EndpointID:         state.id,
			EndpointCreatorID:  state.creatorID,
			EndpointRowStatus:  state.rowStatus,
			EndpointVisibility: state.visibility,
		}
		if err := store.ValidateMemoRelationEndpointRead(snapshot); err != nil {
			return err
		}
	}
	return nil
}

func getSQLiteRelationMemoState(ctx context.Context, executor dbExecutor, memoID int32) (*sqliteRelationMemoState, error) {
	state := &sqliteRelationMemoState{}
	if err := executor.QueryRowContext(ctx, `SELECT id, creator_id, row_status, visibility
		FROM memo WHERE id = ? AND deleted_ts IS NULL`, memoID).Scan(
		&state.id, &state.creatorID, &state.rowStatus, &state.visibility,
	); err != nil {
		return nil, err
	}
	return state, nil
}

func sqliteRelationAuthorizationEndpointIDs(mutation *store.MemoMutation) []int32 {
	unique := make(map[int32]struct{}, len(mutation.ReferenceRelations)+1)
	if mutation.MemoCreate == nil && mutation.MemoID > 0 {
		unique[mutation.MemoID] = struct{}{}
	} else if mutation.CommentContextMemoID != nil {
		unique[*mutation.CommentContextMemoID] = struct{}{}
	}
	for _, relation := range mutation.ReferenceRelations {
		if relation != nil {
			unique[relation.RelatedMemoID] = struct{}{}
		}
	}
	ids := make([]int32, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
