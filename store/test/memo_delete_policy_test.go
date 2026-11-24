package test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/internal/testutil/legacyspace"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

func TestDeleteMemoWithPolicyDeletesOnlyTargetAndIncidentResources(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	contextAuthor, err := ts.CreateUser(ctx, &store.User{Username: "delete-context-author", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	commentAuthor, err := ts.CreateUser(ctx, &store.User{Username: "delete-comment-author", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	replyAuthor, err := ts.CreateUser(ctx, &store.User{Username: "delete-reply-author", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)

	contextMemo, err := ts.CreateMemo(ctx, &store.Memo{UID: "delete-context", CreatorID: contextAuthor.ID, Content: "context", Visibility: store.Public})
	require.NoError(t, err)
	comment := seedLegacyComment(ctx, t, ts, &store.Memo{UID: "delete-comment", CreatorID: commentAuthor.ID, Content: "comment", Visibility: store.Protected}, contextMemo.ID)
	reply := seedLegacyComment(ctx, t, ts, &store.Memo{UID: "delete-reply", CreatorID: replyAuthor.ID, Content: "reply", Visibility: store.Public}, comment.ID)

	attachment, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: "delete-comment-attachment", CreatorID: commentAuthor.ID, Filename: "comment.txt", Type: "text/plain", Size: 1, Blob: []byte("x"), MemoID: &comment.ID,
	})
	require.NoError(t, err)
	reaction, err := ts.UpsertReaction(ctx, &store.Reaction{CreatorID: contextAuthor.ID, MemoID: comment.ID, ReactionType: "heart"})
	require.NoError(t, err)
	share, err := ts.CreateMemoShare(ctx, &store.MemoShare{UID: "delete-comment-share", MemoID: comment.ID, CreatorID: commentAuthor.ID})
	require.NoError(t, err)
	inbox, err := ts.CreateInbox(ctx, &store.Inbox{
		SenderID: commentAuthor.ID, ReceiverID: contextAuthor.ID, Status: store.UNREAD,
		Message: &storepb.InboxMessage{
			Type: storepb.InboxMessage_MEMO_COMMENT,
			Payload: &storepb.InboxMessage_MemoComment{MemoComment: &storepb.InboxMessage_MemoCommentPayload{
				MemoId: comment.ID, RelatedMemoId: contextMemo.ID,
			}},
		},
	})
	require.NoError(t, err)

	_, err = ts.DeleteMemoWithPolicy(ctx, &store.DeleteMemoWithPolicy{MemoID: comment.ID, ActorUserID: replyAuthor.ID})
	require.ErrorIs(t, err, store.ErrMemoPermissionDenied)

	result, err := ts.DeleteMemoWithPolicy(ctx, &store.DeleteMemoWithPolicy{MemoID: comment.ID, ActorUserID: commentAuthor.ID})
	require.NoError(t, err)
	require.True(t, result.ActorCanRead)

	deleted, err := ts.GetMemo(ctx, &store.FindMemo{ID: &comment.ID})
	require.NoError(t, err)
	require.Nil(t, deleted)
	deletedReply, err := ts.GetMemo(ctx, &store.FindMemo{ID: &reply.ID})
	require.NoError(t, err)
	require.Nil(t, deletedReply, "a reply is part of the deleted memo's comment subtree")
	survivor, err := ts.GetMemo(ctx, &store.FindMemo{ID: &contextMemo.ID})
	require.NoError(t, err)
	require.NotNil(t, survivor)

	gotAttachment, err := ts.GetAttachment(ctx, &store.FindAttachment{ID: &attachment.ID})
	require.NoError(t, err)
	require.Nil(t, gotAttachment)
	gotReaction, err := ts.GetReaction(ctx, &store.FindReaction{ID: &reaction.ID})
	require.NoError(t, err)
	require.Nil(t, gotReaction)
	gotShare, err := ts.GetMemoShare(ctx, &store.FindMemoShare{ID: &share.ID})
	require.NoError(t, err)
	require.Nil(t, gotShare)
	gotInboxes, err := ts.ListInboxes(ctx, &store.FindInbox{ID: &inbox.ID})
	require.NoError(t, err)
	require.Len(t, gotInboxes, 1, "Memo deletion must not control the independent inbox lifecycle")

	incident, err := ts.ListMemoRelations(ctx, &store.FindMemoRelation{MemoIDList: []int32{comment.ID}})
	require.NoError(t, err)
	require.Empty(t, incident)
	replyRelations, err := ts.ListMemoRelations(ctx, &store.FindMemoRelation{MemoID: &reply.ID})
	require.NoError(t, err)
	require.Empty(t, replyRelations, "the deleted reply's context relation is removed with it")
}

func TestDeleteMemoWithPolicyDeletesWholeCommentSubtree(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	author, err := ts.CreateUser(ctx, &store.User{Username: "delete-subtree-author", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	commenter, err := ts.CreateUser(ctx, &store.User{Username: "delete-subtree-commenter", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)

	parent, err := ts.CreateMemo(ctx, &store.Memo{UID: "delete-subtree-parent", CreatorID: author.ID, Content: "parent", Visibility: store.Public})
	require.NoError(t, err)
	comment := seedLegacyComment(ctx, t, ts, &store.Memo{UID: "delete-subtree-comment", CreatorID: commenter.ID, Content: "comment", Visibility: store.Public}, parent.ID)
	reply := seedLegacyComment(ctx, t, ts, &store.Memo{UID: "delete-subtree-reply", CreatorID: author.ID, Content: "reply", Visibility: store.Public}, comment.ID)
	unrelated, err := ts.CreateMemo(ctx, &store.Memo{UID: "delete-subtree-unrelated", CreatorID: commenter.ID, Content: "unrelated", Visibility: store.Public})
	require.NoError(t, err)

	result, err := ts.DeleteMemoWithPolicy(ctx, &store.DeleteMemoWithPolicy{MemoID: parent.ID, ActorUserID: author.ID})
	require.NoError(t, err)
	require.True(t, result.ActorCanRead)

	for _, deletedID := range []int32{parent.ID, comment.ID, reply.ID} {
		deleted, err := ts.GetMemo(ctx, &store.FindMemo{ID: &deletedID})
		require.NoError(t, err)
		require.Nil(t, deleted, "memo %d should be deleted with the parent", deletedID)
	}
	survivor, err := ts.GetMemo(ctx, &store.FindMemo{ID: &unrelated.ID})
	require.NoError(t, err)
	require.NotNil(t, survivor)

	remaining, err := ts.ListMemos(ctx, &store.FindMemo{CreatorID: &commenter.ID, ExcludeComments: true})
	require.NoError(t, err)
	for _, memo := range remaining {
		require.NotEqual(t, comment.ID, memo.ID, "a deleted comment must not resurface as a top-level memo")
	}
}

func TestMemoDeleteActorCanReadAudienceMatrix(t *testing.T) {
	tests := []struct {
		name         string
		rowStatus    store.RowStatus
		visibility   store.Visibility
		actorCanRead bool
	}{
		{name: "public", rowStatus: store.Normal, visibility: store.Public, actorCanRead: true},
		{name: "protected", rowStatus: store.Normal, visibility: store.Protected, actorCanRead: true},
		{name: "private", rowStatus: store.Normal, visibility: store.Private, actorCanRead: true},
		{name: "legacy workspace audience", rowStatus: store.Normal, visibility: store.SpaceAudience, actorCanRead: true},
		{name: "unknown audience", rowStatus: store.Normal, visibility: store.Visibility("UNKNOWN")},
		{name: "invalid lifecycle", rowStatus: store.RowStatus("UNKNOWN"), visibility: store.Public},
		{name: "archived author", rowStatus: store.Archived, visibility: store.Private, actorCanRead: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.actorCanRead, store.MemoDeleteActorCanRead(test.rowStatus, test.visibility))
		})
	}
}

func TestDeleteMemoWithPolicyUsesLegacyMemoAuthorWithoutMembership(t *testing.T) {
	ctx := t.Context()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })

	author, err := ts.CreateUser(ctx, &store.User{Username: "delete-legacy-author", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	peer, err := ts.CreateUser(ctx, &store.User{Username: "delete-legacy-peer", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	sqlite := ts.GetDriver().GetDB()
	spaceID, err := legacyspace.Seed(ctx, sqlite, "delete-legacy-space", "Legacy Workspace", peer.ID)
	require.NoError(t, err)

	for index, placement := range []any{spaceID, int32(2147483000), nil} {
		memo, err := ts.CreateMemo(ctx, &store.Memo{
			UID:        fmt.Sprintf("delete-legacy-note-%d", index),
			CreatorID:  author.ID,
			Content:    "Legacy memo",
			Visibility: store.Private,
		})
		require.NoError(t, err)
		_, err = sqlite.ExecContext(ctx, `UPDATE memo SET visibility = 'SPACE', space_id = ? WHERE id = ?`, placement, memo.ID)
		require.NoError(t, err)

		_, err = ts.DeleteMemoWithPolicy(ctx, &store.DeleteMemoWithPolicy{MemoID: memo.ID, ActorUserID: peer.ID})
		require.ErrorIs(t, err, store.ErrMemoPermissionDenied)
		result, err := ts.DeleteMemoWithPolicy(ctx, &store.DeleteMemoWithPolicy{MemoID: memo.ID, ActorUserID: author.ID})
		require.NoError(t, err)
		require.True(t, result.ActorCanRead)
	}
}
