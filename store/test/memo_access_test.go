package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestMemoAccessScopeRequiresActiveAuthor(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "access-owner-only-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	viewer, err := ts.CreateUser(ctx, &store.User{Username: "access-owner-only-viewer", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	memo, err := ts.CreateMemo(ctx, &store.Memo{UID: "access-owner-only-memo", CreatorID: owner.ID, Content: "memo", Visibility: store.Public})
	require.NoError(t, err)
	archived := store.Archived

	ownerRows, err := ts.ListMemos(ctx, &store.FindMemo{
		Access: &store.MemoAccessScope{UserID: &owner.ID},
	})
	require.NoError(t, err)
	require.Equal(t, []int32{memo.ID}, memoIDs(ownerRows))
	viewerRows, err := ts.ListMemos(ctx, &store.FindMemo{
		Access: &store.MemoAccessScope{UserID: &viewer.ID},
	})
	require.NoError(t, err)
	require.Empty(t, viewerRows)
	anonymousRows, err := ts.ListMemos(ctx, &store.FindMemo{Access: &store.MemoAccessScope{}})
	require.NoError(t, err)
	require.Empty(t, anonymousRows)

	_, err = ts.UpdateUser(ctx, &store.UpdateUser{ID: owner.ID, RowStatus: &archived})
	require.NoError(t, err)
	inactiveRows, err := ts.ListMemos(ctx, &store.FindMemo{
		Access: &store.MemoAccessScope{UserID: &owner.ID},
	})
	require.NoError(t, err)
	require.Empty(t, inactiveRows)
}

func TestMemoAccessScopeIgnoresFoldedFilterOperands(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "access-folded-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	other, err := ts.CreateUser(ctx, &store.User{Username: "access-folded-other", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)

	_, err = ts.CreateMemo(ctx, &store.Memo{UID: "access-folded-private", CreatorID: owner.ID, Content: "private", Visibility: store.Private})
	require.NoError(t, err)
	public, err := ts.CreateMemo(ctx, &store.Memo{UID: "access-folded-public", CreatorID: owner.ID, Content: "public", Visibility: store.Public})
	require.NoError(t, err)
	_, err = ts.CreateMemo(ctx, &store.Memo{UID: "access-folded-public-other", CreatorID: other.ID, Content: "other", Visibility: store.Public})
	require.NoError(t, err)

	// Each filter folds an operand away. Its arguments must go with it, or they
	// would bind to the access-scope placeholders that follow.
	for _, memoFilter := range []string{
		fmt.Sprintf(`(creator_id == %d || true) && visibility == "PUBLIC"`, owner.ID),
		fmt.Sprintf(`(creator_id == %d && false) || visibility == "PUBLIC"`, owner.ID),
		fmt.Sprintf(`!(creator_id == %d || true) || visibility == "PUBLIC"`, owner.ID),
	} {
		memos, err := ts.ListMemos(ctx, &store.FindMemo{
			Filters: []string{memoFilter},
			Access:  &store.MemoAccessScope{UserID: &owner.ID},
		})
		require.NoError(t, err, memoFilter)
		require.Equal(t, []int32{public.ID}, memoIDs(memos), memoFilter)
	}
}

func memoIDs(memos []*store.Memo) []int32 {
	ids := make([]int32, 0, len(memos))
	for _, memo := range memos {
		ids = append(ids, memo.ID)
	}
	return ids
}
