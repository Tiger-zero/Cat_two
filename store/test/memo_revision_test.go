package test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestMemoRevisionConcurrentWrites(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	owner, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	signature := sha256.Sum256([]byte("original request"))
	idempotencyDigest := hex.EncodeToString(signature[:])
	created := &store.Memo{
		UID: "memo-revision", CreatorID: owner.ID, Content: "original", Visibility: store.Private,
		RowStatus: store.Archived, Pinned: true, CreationRequestDigest: idempotencyDigest,
	}
	require.NoError(t, ts.ApplyMemoMutation(ctx, &store.MemoMutation{
		MemoCreate: created, PreserveInitialState: true,
	}))
	require.EqualValues(t, 1, created.Revision)
	read, err := ts.GetMemo(ctx, &store.FindMemo{ID: &created.ID, ExcludeContent: true})
	require.NoError(t, err)
	require.Equal(t, store.Archived, read.RowStatus)
	require.True(t, read.Pinned)
	require.Equal(t, idempotencyDigest, read.CreationRequestDigest)

	results := make(chan error, 2)
	start := make(chan struct{})
	for _, content := range []string{"writer A", "writer B"} {
		go func(content string) {
			<-start
			results <- ts.UpdateMemo(ctx, &store.UpdateMemo{
				ID: created.ID, Content: &content, ExpectedRevision: &created.Revision,
			})
		}(content)
	}
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		switch err := <-results; {
		case err == nil:
			successes++
		case errors.Is(err, store.ErrMemoRevisionConflict):
			conflicts++
		default:
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	read, err = ts.GetMemo(ctx, &store.FindMemo{ID: &created.ID})
	require.NoError(t, err)
	require.Contains(t, []string{"writer A", "writer B"}, read.Content)
	require.EqualValues(t, 2, read.Revision)
	require.Equal(t, idempotencyDigest, read.CreationRequestDigest)
}

func TestMemoRevisionTracksBindingsAndLifecycle(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	owner, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	memo, err := ts.CreateMemo(ctx, &store.Memo{
		UID: "memo-revision-attachments", CreatorID: owner.ID, Content: "body", Visibility: store.Private,
	})
	require.NoError(t, err)
	attachment, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: "revision-attachment", CreatorID: owner.ID, Filename: "initial.txt", Type: "text/plain",
	})
	require.NoError(t, err)

	readRevision := func(trash store.MemoTrashScope) int64 {
		t.Helper()
		item, err := ts.GetMemo(ctx, &store.FindMemo{ID: &memo.ID, Trash: trash})
		require.NoError(t, err)
		require.Equal(t, memo.Content, item.Content)
		return item.Revision
	}
	mutation := &store.MemoMutation{
		MemoID: memo.ID, MemoCreatorID: owner.ID, ExpectedMemoContent: memo.Content,
		ExpectedRevision: new(int64(1)), Bindings: []*store.MemoAttachmentBinding{
			{ID: attachment.ID, UID: attachment.UID, UpdatedTs: attachment.CreatedTs},
		},
	}
	require.NoError(t, ts.ApplyMemoMutation(ctx, mutation))
	require.EqualValues(t, 2, readRevision(store.MemoTrashExclude))

	require.NoError(t, ts.UpdateAttachment(ctx, &store.UpdateAttachment{
		ID: attachment.ID, Filename: new("replaced.txt"),
		Content: &store.AttachmentContent{Type: "text/plain", Blob: []byte("new")},
		Policy:  &store.MemoWritePolicy{ActorUserID: owner.ID, Personal: true},
	}))
	require.EqualValues(t, 3, readRevision(store.MemoTrashExclude))
	mutation.ExpectedRevision = new(int64(2))
	mutation.Bindings[0].WasBoundToMemo = true
	require.ErrorIs(t, ts.ApplyMemoMutation(ctx, mutation), store.ErrMemoRevisionConflict)
	require.EqualValues(t, 3, readRevision(store.MemoTrashExclude))

	require.NoError(t, ts.UpdateMemo(ctx, &store.UpdateMemo{
		ID: memo.ID, ExpectedRevision: new(int64(3)), RowStatus: new(store.Archived),
	}))
	require.EqualValues(t, 4, readRevision(store.MemoTrashExclude))
	now := time.Now().Unix()
	trash := store.MemoTrashMutation{MemoID: memo.ID, ActorUserID: owner.ID, Action: store.MemoTrashMove, NowTs: now}
	_, err = ts.ApplyMemoTrash(ctx, &trash)
	require.NoError(t, err)
	require.EqualValues(t, 5, readRevision(store.MemoTrashOnly))
	trash.Action = store.MemoTrashRestore
	_, err = ts.ApplyMemoTrash(ctx, &trash)
	require.NoError(t, err)
	require.EqualValues(t, 6, readRevision(store.MemoTrashExclude))
}
