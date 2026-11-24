package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestMemoTrashRestoresOnlyTheSelectedBatch(t *testing.T) {
	ctx := t.Context()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	owner, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	root, err := ts.CreateMemo(ctx, &store.Memo{UID: "trash-batch-root", CreatorID: owner.ID, Content: "root", Visibility: store.Private})
	require.NoError(t, err)
	comment, err := ts.CreateMemoComment(ctx, &store.Memo{UID: "trash-batch-comment", CreatorID: owner.ID, Content: "comment", Visibility: store.Private}, root.ID, owner.ID)
	require.NoError(t, err)
	reply, err := ts.CreateMemoComment(ctx, &store.Memo{UID: "trash-batch-reply", CreatorID: owner.ID, Content: "reply", Visibility: store.Private}, comment.ID, owner.ID)
	require.NoError(t, err)

	now := time.Now().Unix()
	change := func(memo *store.Memo, action store.MemoTrashAction) error {
		_, err := ts.ApplyMemoTrash(ctx, &store.MemoTrashMutation{MemoID: memo.ID, ActorUserID: owner.ID, Action: action, NowTs: now})
		return err
	}
	require.NoError(t, change(reply, store.MemoTrashMove))
	require.NoError(t, change(root, store.MemoTrashMove))
	require.ErrorIs(t, change(reply, store.MemoTrashRestore), store.ErrMemoMutationConflict)
	require.NoError(t, change(root, store.MemoTrashRestore))
	for _, memo := range []*store.Memo{root, comment} {
		stored, err := ts.GetMemo(ctx, &store.FindMemo{ID: &memo.ID})
		require.NoError(t, err)
		require.NotNil(t, stored)
	}
	stored, err := ts.GetMemo(ctx, &store.FindMemo{ID: &reply.ID})
	require.NoError(t, err)
	require.Nil(t, stored)
	require.NoError(t, change(reply, store.MemoTrashRestore))
	stored, err = ts.GetMemo(ctx, &store.FindMemo{ID: &reply.ID})
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, comment.UID, *stored.ParentUID)
}

func TestMemoTrashKeepsSurvivingReplyAfterOwnerDeletion(t *testing.T) {
	ctx := t.Context()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	parentOwner, err := ts.CreateUser(ctx, &store.User{Username: "trash-root-owner", Role: store.RoleUser})
	require.NoError(t, err)
	replyOwner, err := ts.CreateUser(ctx, &store.User{Username: "trash-reply-owner", Role: store.RoleUser})
	require.NoError(t, err)
	parent, err := ts.CreateMemo(ctx, &store.Memo{UID: "trash-legacy-parent", CreatorID: parentOwner.ID, Content: "parent", Visibility: store.Public})
	require.NoError(t, err)
	reply := seedLegacyComment(ctx, t, ts, &store.Memo{
		UID: "trash-legacy-reply", CreatorID: replyOwner.ID, Content: "reply", Visibility: store.Public,
	}, parent.ID)
	now := time.Now().Unix()
	_, err = ts.ApplyMemoTrash(ctx, &store.MemoTrashMutation{MemoID: parent.ID, ActorUserID: parentOwner.ID, Action: store.MemoTrashMove, NowTs: now})
	require.NoError(t, err)
	_, err = ts.DeleteUser(ctx, &store.DeleteUser{ID: parentOwner.ID})
	require.NoError(t, err)
	recoverable, err := ts.ListMemos(ctx, &store.FindMemo{CreatorID: &replyOwner.ID, Trash: store.MemoTrashOnly})
	require.NoError(t, err)
	require.Len(t, recoverable, 1)
	require.Equal(t, reply.ID, recoverable[0].ID)
	_, err = ts.ApplyMemoTrash(ctx, &store.MemoTrashMutation{MemoID: reply.ID, ActorUserID: replyOwner.ID, Action: store.MemoTrashRestore, NowTs: now})
	require.NoError(t, err)
	restored, err := ts.GetMemo(ctx, &store.FindMemo{ID: &reply.ID})
	require.NoError(t, err)
	require.NotNil(t, restored)
	require.Equal(t, reply.Content, restored.Content)
	require.Nil(t, restored.ParentUID)
}

func TestMemoTrashRestoreAndExpiryAreAtomic(t *testing.T) {
	ctx := t.Context()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	owner, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	memo, err := ts.CreateMemo(ctx, &store.Memo{UID: "trash-race-memo", CreatorID: owner.ID, Content: "keep unless purged", Visibility: store.Private})
	require.NoError(t, err)
	deletedAt := time.Now().Add(-store.MemoTrashRetention)
	_, err = ts.ApplyMemoTrash(ctx, &store.MemoTrashMutation{MemoID: memo.ID, ActorUserID: owner.ID, Action: store.MemoTrashMove, NowTs: deletedAt.Unix()})
	require.NoError(t, err)

	cutoff := deletedAt.Add(store.MemoTrashRetention)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, mutation := range []store.MemoTrashMutation{
		{MemoID: memo.ID, ActorUserID: owner.ID, Action: store.MemoTrashRestore, NowTs: cutoff.Add(-time.Second).Unix()},
		{MemoID: memo.ID, Action: store.MemoTrashPurge, NowTs: cutoff.Unix()},
	} {
		go func() {
			<-start
			_, err := ts.ApplyMemoTrash(ctx, &mutation)
			results <- err
		}()
	}
	close(start)
	first, second := <-results, <-results
	if first == nil {
		require.ErrorIs(t, second, store.ErrMemoMutationConflict)
	} else {
		require.ErrorIs(t, first, store.ErrMemoMutationConflict)
		require.NoError(t, second)
	}
	stored, err := ts.GetMemo(ctx, &store.FindMemo{ID: &memo.ID, Trash: store.MemoTrashInclude})
	require.NoError(t, err)
	if stored != nil {
		require.Nil(t, stored.DeletedTs)
		require.Equal(t, memo.Content, stored.Content)
	}
}
