package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestMemoWritePolicyUsesCurrentAudienceWhenCreatingShare(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "memo-current-share-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	memo, err := ts.CreateMemo(ctx, &store.Memo{
		UID: "memo-current-share", CreatorID: owner.ID, Content: "memo", Visibility: store.Public,
	})
	require.NoError(t, err)

	policy := &store.MemoWritePolicy{ActorUserID: owner.ID, CreatingShare: true}
	spaceAudience := store.SpaceAudience
	require.NoError(t, ts.UpdateMemo(ctx, &store.UpdateMemo{ID: memo.ID, Visibility: &spaceAudience}))
	_, err = ts.CreateMemoShare(ctx, &store.MemoShare{
		UID: "memo-current-share-token", MemoID: memo.ID, CreatorID: owner.ID, Policy: policy,
	})
	require.ErrorIs(t, err, store.ErrMemoShareConflict)

	public := store.Public
	archived := store.Archived
	require.NoError(t, ts.UpdateMemo(ctx, &store.UpdateMemo{ID: memo.ID, Visibility: &public, RowStatus: &archived}))
	_, err = ts.CreateMemoShare(ctx, &store.MemoShare{
		UID: "memo-current-archived-share", MemoID: memo.ID, CreatorID: owner.ID, Policy: policy,
	})
	require.ErrorIs(t, err, store.ErrMemoMutationConflict)
}

func memoWritePolicy(actorUserID int32) *store.MemoWritePolicy {
	return &store.MemoWritePolicy{ActorUserID: actorUserID}
}
