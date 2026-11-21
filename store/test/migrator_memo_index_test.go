package test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestMigrationMemoOwnerPinnedCreatedIndex(t *testing.T) {
	t.Parallel()
	if getDriverFromEnv() != "sqlite" {
		t.Skip("the owner feed partial index is SQLite-specific")
	}
	ctx := t.Context()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	db := ts.GetDriver().GetDB()
	requireIndex := func() {
		t.Helper()
		var partial int
		require.NoError(t, db.QueryRowContext(ctx, `SELECT partial FROM pragma_index_list('memo')
			WHERE name = 'idx_memo_owner_pinned_created'`).Scan(&partial))
		require.Equal(t, 1, partial)
	}
	requireIndex()

	owner, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	other, err := createTestingUserWithRole(ctx, ts, "legacy-owner", store.RoleUser)
	require.NoError(t, err)
	create := func(uid string, creatorID int32, createdTs int64) *store.Memo {
		t.Helper()
		memo, err := ts.CreateMemo(ctx, &store.Memo{UID: uid, CreatorID: creatorID, CreatedTs: createdTs, Visibility: store.Private})
		require.NoError(t, err)
		return memo
	}
	pinned := create("feed-pinned", owner.ID, 100)
	olderID := create("feed-lower-id", owner.ID, 200)
	newerID := create("feed-higher-id", owner.ID, 200)
	archived := create("feed-archived", owner.ID, 300)
	deleted := create("feed-trashed", owner.ID, 400)
	create("feed-other-owner", other.ID, 500)

	require.NoError(t, ts.UpdateMemo(ctx, &store.UpdateMemo{ID: pinned.ID, Pinned: new(true)}))
	require.NoError(t, ts.UpdateMemo(ctx, &store.UpdateMemo{ID: archived.ID, RowStatus: new(store.Archived)}))
	_, err = ts.ApplyMemoTrash(ctx, &store.MemoTrashMutation{MemoID: deleted.ID, ActorUserID: owner.ID, Action: store.MemoTrashMove, NowTs: 600})
	require.NoError(t, err)
	_, err = ts.CreateMemoComment(ctx, &store.Memo{UID: "feed-comment", CreatorID: owner.ID, CreatedTs: 700, Visibility: store.Private}, pinned.ID, owner.ID)
	require.NoError(t, err)

	requireFeed := func() {
		t.Helper()
		find := &store.FindMemo{
			Access:          &store.MemoAccessScope{UserID: &owner.ID, OwnerOnly: true},
			RowStatus:       new(store.Normal),
			ExcludeComments: true,
			OrderByPinned:   true,
			Limit:           new(2),
		}
		page, err := ts.ListMemos(ctx, find)
		require.NoError(t, err)
		require.Len(t, page, 2)
		require.Equal(t, pinned.ID, page[0].ID)
		require.Equal(t, newerID.ID, page[1].ID, "equal timestamps retain descending ID order")

		find.Offset = new(2)
		page, err = ts.ListMemos(ctx, find)
		require.NoError(t, err)
		require.Len(t, page, 1)
		require.Equal(t, olderID.ID, page[0].ID)

		find.Offset = nil
		find.RowStatus = new(store.Archived)
		page, err = ts.ListMemos(ctx, find)
		require.NoError(t, err)
		require.Len(t, page, 1)
		require.Equal(t, archived.ID, page[0].ID)
	}
	requireFeed()

	removeSchemaAfterMCPConnection(ctx, t, ts, "sqlite")
	setSchemaVersion(ctx, t, ts, "26.11.4")
	require.NoError(t, ts.Migrate(ctx))
	requireIndex()
	requireFeed()
}
