package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

// seedLegacyComment inserts a comment relationship that may predate owner-only participation.
func seedLegacyComment(ctx context.Context, t *testing.T, ts *store.Store, memo *store.Memo, parentID int32) *store.Memo {
	t.Helper()
	comment, err := ts.CreateMemo(ctx, memo)
	require.NoError(t, err)
	_, err = ts.GetDriver().GetDB().ExecContext(ctx,
		`INSERT INTO memo_relation (memo_id, related_memo_id, type) VALUES (?, ?, 'COMMENT')`, comment.ID, parentID)
	require.NoError(t, err)
	return comment
}

func TestDeleteUserRemovesAuthoredMemoWithoutFollowingComments(t *testing.T) {
	ctx := t.Context()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })

	user, err := ts.CreateUser(ctx, &store.User{Username: "delete-comment-user", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	peer, err := ts.CreateUser(ctx, &store.User{Username: "delete-comment-peer", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	contextMemo, err := ts.CreateMemo(ctx, &store.Memo{UID: "peer-context", CreatorID: peer.ID, Content: "context", Visibility: store.Public})
	require.NoError(t, err)
	comment := seedLegacyComment(ctx, t, ts, &store.Memo{
		UID: "user-comment", CreatorID: user.ID, Content: "comment", Visibility: store.Public,
	}, contextMemo.ID)
	reply := seedLegacyComment(ctx, t, ts, &store.Memo{
		UID: "peer-reply", CreatorID: peer.ID, Content: "reply", Visibility: store.Public,
	}, comment.ID)

	_, err = ts.DeleteUser(ctx, &store.DeleteUser{ID: user.ID})
	require.NoError(t, err)
	deletedUser, err := ts.GetUser(ctx, &store.FindUser{ID: &user.ID})
	require.NoError(t, err)
	require.Nil(t, deletedUser)
	deletedComment, err := ts.GetMemo(ctx, &store.FindMemo{ID: &comment.ID})
	require.NoError(t, err)
	require.Nil(t, deletedComment)
	for _, memo := range []*store.Memo{contextMemo, reply} {
		remaining, getErr := ts.GetMemo(ctx, &store.FindMemo{ID: &memo.ID})
		require.NoError(t, getErr)
		require.NotNil(t, remaining)
	}
	relations, err := ts.ListMemoRelations(ctx, &store.FindMemoRelation{MemoIDList: []int32{comment.ID}})
	require.NoError(t, err)
	require.Empty(t, relations)
}
