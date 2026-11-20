package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/store"
)

func TestMoveMemoPreservesIndependentConversation(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	author, err := ts.CreateRegularUser(ctx, "move-author")
	require.NoError(t, err)
	commenter, err := ts.CreateRegularUser(ctx, "move-commenter")
	require.NoError(t, err)
	reader, err := ts.CreateRegularUser(ctx, "move-reader")
	require.NoError(t, err)
	authorCtx := ts.CreateUserContext(ctx, author.ID)
	commenterCtx := ts.CreateUserContext(ctx, commenter.ID)
	readerCtx := ts.CreateUserContext(ctx, reader.ID)
	source, err := ts.Store.CreateSpace(ctx, &store.Space{UID: "move-source", Title: "Source"}, author.ID)
	require.NoError(t, err)
	target, err := ts.Store.CreateSpace(ctx, &store.Space{UID: "move-target", Title: "Target"}, author.ID)
	require.NoError(t, err)
	for _, member := range []*store.SpaceMember{
		{SpaceID: source.ID, UserID: commenter.ID, Role: store.SpaceMemberRoleUser},
		{SpaceID: target.ID, UserID: reader.ID, Role: store.SpaceMemberRoleUser},
	} {
		_, err := ts.InviteAndAcceptSpaceMember(ctx, member, author.ID)
		require.NoError(t, err)
	}
	sourceName, targetName := "spaces/"+source.UID, "spaces/"+target.UID
	reference, err := ts.Service.CreateMemo(authorCtx, &apipb.CreateMemoRequest{Memo: &apipb.Memo{
		Content: "source reference", Space: &sourceName, Visibility: apipb.Visibility_SPACE,
	}})
	require.NoError(t, err)
	attachment, err := ts.Service.CreateAttachment(authorCtx, &apipb.CreateAttachmentRequest{
		Attachment: &apipb.Attachment{Filename: "proposal.txt", Type: "text/plain", Content: []byte("proposal attachment")},
	})
	require.NoError(t, err)
	original, err := ts.Service.CreateMemo(authorCtx, &apipb.CreateMemoRequest{Memo: &apipb.Memo{
		Content: "original memo", Space: &sourceName, Visibility: apipb.Visibility_SPACE,
		Attachments: []*apipb.Attachment{attachment},
		Relations:   []*apipb.MemoRelation{{Type: apipb.MemoRelation_REFERENCE, RelatedMemo: &apipb.MemoRelation_MemoRef{Name: reference.Name}}},
	}})
	require.NoError(t, err)
	comment, err := ts.Service.CreateMemoComment(authorCtx, &apipb.CreateMemoCommentRequest{
		Parent: original.Name, Comment: &apipb.Memo{Content: "source comment", Space: &sourceName, Visibility: apipb.Visibility_SPACE},
	})
	require.NoError(t, err)
	reaction, err := ts.Service.UpsertMemoReaction(authorCtx, &apipb.UpsertMemoReactionRequest{
		Parent: original.Name, Reaction: &apipb.Reaction{ReactionType: "👍"},
	})
	require.NoError(t, err)

	moved, err := ts.Service.UpdateMemo(authorCtx, &apipb.UpdateMemoRequest{
		Memo:       &apipb.Memo{Name: original.Name, Space: &targetName, Visibility: apipb.Visibility_SPACE},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"space", "visibility"}},
	})
	require.NoError(t, err)
	require.Equal(t, original.Name, moved.Name)
	require.Equal(t, original.Creator, moved.Creator)
	require.Equal(t, original.CreateTime, moved.CreateTime)
	require.Equal(t, original.UpdateTime, moved.UpdateTime)
	require.Equal(t, original.Content, moved.Content)
	require.Len(t, moved.Attachments, 1)
	require.Equal(t, original.Attachments, moved.Attachments)
	require.Len(t, moved.Relations, 2, "the author in both Spaces retains references and comments")
	require.Len(t, moved.Reactions, 1)
	require.Equal(t, reaction.Name, moved.Reactions[0].Name)

	for _, viewer := range []context.Context{commenterCtx, readerCtx} {
		_, err = ts.Service.GetMemo(viewer, &apipb.GetMemoRequest{Name: original.Name})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = ts.Service.GetMemo(viewer, &apipb.GetMemoRequest{Name: comment.Name})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
		_, err = ts.Service.ListMemoComments(viewer, &apipb.ListMemoCommentsRequest{Parent: original.Name})
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	}

	readableComment, err := ts.Service.GetMemo(authorCtx, &apipb.GetMemoRequest{Name: comment.Name})
	require.NoError(t, err)
	require.Equal(t, original.Name, readableComment.GetParent())
	require.Equal(t, sourceName, readableComment.GetSpace(), "the comment keeps its independent placement")
	require.Equal(t, apipb.Visibility_PRIVATE, readableComment.Visibility)
	require.Equal(t, comment.Content, readableComment.Content)
	comments, err := ts.Service.ListMemoComments(authorCtx, &apipb.ListMemoCommentsRequest{Parent: original.Name})
	require.NoError(t, err)
	require.Len(t, comments.Comments, 1)

	_, err = ts.Service.DeleteMemoReaction(readerCtx, &apipb.DeleteMemoReactionRequest{Name: reaction.Name})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = ts.Service.DeleteMemoReaction(authorCtx, &apipb.DeleteMemoReactionRequest{Name: reaction.Name})
	require.NoError(t, err)
	reactions, err := ts.Service.ListMemoReactions(authorCtx, &apipb.ListMemoReactionsRequest{Parent: original.Name})
	require.NoError(t, err)
	require.Empty(t, reactions.Reactions)
}
