package test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/usememos/memos/proto/gen/api"
)

func TestCreateMemoReplayChecksOriginalRequestAndOwner(t *testing.T) {
	ctx := t.Context()
	ts := NewTestService(t)
	t.Cleanup(ts.Cleanup)
	owner, err := ts.CreateRegularUser(ctx, "replay-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	other, err := ts.CreateRegularUser(ctx, "replay-other")
	require.NoError(t, err)
	attachment, err := ts.Service.CreateAttachment(ownerCtx, &apipb.CreateAttachmentRequest{
		Attachment: &apipb.Attachment{Filename: "replay.txt", Content: []byte("attachment contents")},
	})
	require.NoError(t, err)
	const memoID = "replay-with-attachment"
	request := &apipb.CreateMemoRequest{
		MemoId: memoID, Idempotent: true,
		Memo: &apipb.Memo{Content: "Original markdown", Attachments: []*apipb.Attachment{{Name: attachment.Name}}},
	}
	first, err := ts.Service.CreateMemo(ownerCtx, request)
	require.NoError(t, err)
	require.Len(t, first.Attachments, 1)
	require.NotEmpty(t, first.Revision)
	updated, err := ts.Service.UpdateMemo(ownerCtx, &apipb.UpdateMemoRequest{
		Memo:       &apipb.Memo{Name: first.Name, Content: "A later edit"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}}, ExpectedRevision: first.Revision,
	})
	require.NoError(t, err)
	replayed, err := ts.Service.CreateMemo(ownerCtx, proto.CloneOf(request))
	require.NoError(t, err)
	require.Equal(t, first.Name, replayed.Name)
	require.Equal(t, updated.Revision, replayed.Revision)
	require.Equal(t, "A later edit", replayed.Content)
	require.Equal(t, attachment.Name, replayed.GetAttachments()[0].Name)

	different := proto.CloneOf(request)
	different.Memo.Content = "The same ID for another operation"
	_, err = ts.Service.CreateMemo(ownerCtx, different)
	require.Equal(t, codes.AlreadyExists, status.Code(err))
	_, err = ts.Service.CreateMemo(ts.CreateUserContext(ctx, other.ID), request)
	require.Equal(t, codes.AlreadyExists, status.Code(err))
	_, err = ts.Service.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{MemoId: "  ", Idempotent: true, Memo: request.Memo})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = ts.Service.DeleteMemo(ownerCtx, &apipb.DeleteMemoRequest{Name: first.Name})
	require.NoError(t, err)
	_, err = ts.Service.CreateMemo(ownerCtx, request)
	require.Equal(t, codes.AlreadyExists, status.Code(err))
}

func TestConcurrentCreateMemoReplayWithSameAttachment(t *testing.T) {
	ctx := t.Context()
	ts := NewTestService(t)
	t.Cleanup(ts.Cleanup)
	owner, err := ts.CreateRegularUser(ctx, "concurrent-replay")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	attachment, err := ts.Service.CreateAttachment(ownerCtx, &apipb.CreateAttachmentRequest{
		Attachment: &apipb.Attachment{Filename: "replay.txt", Content: []byte("attachment contents")},
	})
	require.NoError(t, err)
	const memoID = "concurrent-replay"
	request := &apipb.CreateMemoRequest{
		MemoId: memoID, Idempotent: true,
		Memo: &apipb.Memo{Content: "Concurrent markdown", Attachments: []*apipb.Attachment{{Name: attachment.Name}}},
	}
	type result struct {
		memo *apipb.Memo
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for range 2 {
		go func() {
			<-start
			memo, err := ts.Service.CreateMemo(ownerCtx, proto.CloneOf(request))
			results <- result{memo: memo, err: err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	for _, response := range []result{first, second} {
		require.NoError(t, response.err)
		require.Equal(t, "memos/"+memoID, response.memo.Name)
		require.Len(t, response.memo.Attachments, 1)
	}
	feed, err := ts.Service.ListMemos(ownerCtx, &apipb.ListMemosRequest{})
	require.NoError(t, err)
	require.Len(t, feed.Memos, 1)
}

func TestMemoCommentReplayAfterParentIsArchived(t *testing.T) {
	ctx := t.Context()
	ts := NewTestService(t)
	t.Cleanup(ts.Cleanup)
	owner, err := ts.CreateRegularUser(ctx, "comment-replay")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	parent, err := ts.Service.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{Memo: &apipb.Memo{Content: "Parent"}})
	require.NoError(t, err)
	request := &apipb.CreateMemoCommentRequest{
		Parent: parent.Name, CommentId: "retry-comment", Idempotent: true,
		Comment: &apipb.Memo{Content: "Comment"},
	}
	first, err := ts.Service.CreateMemoComment(ownerCtx, request)
	require.NoError(t, err)
	_, err = ts.Service.UpdateMemo(ownerCtx, &apipb.UpdateMemoRequest{
		Memo:       &apipb.Memo{Name: parent.Name, State: apipb.State_ARCHIVED},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}}, ExpectedRevision: parent.Revision,
	})
	require.NoError(t, err)
	replayed, err := ts.Service.CreateMemoComment(ownerCtx, request)
	require.NoError(t, err)
	require.Equal(t, first.Name, replayed.Name)
	_, err = ts.Service.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{
		MemoId: request.CommentId, Idempotent: true, Memo: request.Comment,
	})
	require.Equal(t, codes.AlreadyExists, status.Code(err))
}

func TestUpdateMemoRevisionCoversBodyAndLifecycle(t *testing.T) {
	ctx := t.Context()
	ts := NewTestService(t)
	t.Cleanup(ts.Cleanup)
	owner, err := ts.CreateRegularUser(ctx, "memo-revision")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	first, err := ts.Service.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{Memo: &apipb.Memo{Content: "Original"}})
	require.NoError(t, err)
	archived, err := ts.Service.UpdateMemo(ownerCtx, &apipb.UpdateMemoRequest{
		Memo:       &apipb.Memo{Name: first.Name, State: apipb.State_ARCHIVED},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}}, ExpectedRevision: first.Revision,
	})
	require.NoError(t, err)
	require.NotEqual(t, first.Revision, archived.Revision)
	write := &apipb.UpdateMemoRequest{
		Memo:       &apipb.Memo{Name: first.Name, Content: "Edited"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}}, ExpectedRevision: first.Revision,
	}
	_, err = ts.Service.UpdateMemo(ownerCtx, write)
	require.Equal(t, codes.Aborted, status.Code(err))
	current, err := ts.Service.GetMemo(ownerCtx, &apipb.GetMemoRequest{Name: first.Name})
	require.NoError(t, err)
	require.Equal(t, apipb.State_ARCHIVED, current.State)
	require.Equal(t, "Original", current.Content)
	write.ExpectedRevision = current.Revision
	updated, err := ts.Service.UpdateMemo(ownerCtx, write)
	require.NoError(t, err)
	require.NotEqual(t, archived.Revision, updated.Revision)
	require.Equal(t, "Edited", updated.Content)
}
