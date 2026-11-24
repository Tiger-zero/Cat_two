package test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestPersonalAttachmentScopeUsesMemoOwnerWithoutSpace(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "attachment-scope-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	other, err := ts.CreateUser(ctx, &store.User{Username: "attachment-scope-other", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)

	memo, err := ts.CreateMemo(ctx, &store.Memo{
		UID: "attachment-scope-owned-memo", CreatorID: owner.ID, Content: "owned", Visibility: store.Private,
	})
	require.NoError(t, err)
	otherMemo, err := ts.CreateMemo(ctx, &store.Memo{
		UID: "attachment-scope-other-memo", CreatorID: other.ID, Content: "other", Visibility: store.Public,
	})
	require.NoError(t, err)
	_, err = ts.GetDriver().GetDB().ExecContext(ctx, "UPDATE memo SET space_id = ?, visibility = 'SPACE' WHERE id = ?", 2147483000, memo.ID)
	require.NoError(t, err)

	createAttachment := func(uid string, creatorID int32, memoID *int32) *store.Attachment {
		t.Helper()
		attachment, createErr := ts.CreateAttachment(ctx, &store.Attachment{
			UID: uid, CreatorID: creatorID, Filename: uid + ".txt", Type: "text/plain", MemoID: memoID,
		})
		require.NoError(t, createErr)
		return attachment
	}
	unlinked := createAttachment("attachment-scope-unlinked", owner.ID, nil)
	owned := createAttachment("attachment-scope-owned-file", other.ID, &memo.ID)
	inaccessible := createAttachment("attachment-scope-inaccessible", owner.ID, &otherMemo.ID)

	baseTime := time.Now().Unix() - 100
	for attachment, updatedTs := range map[*store.Attachment]int64{
		inaccessible: baseTime + 30,
		owned:        baseTime + 20,
		unlinked:     baseTime + 10,
	} {
		require.NoError(t, ts.UpdateAttachment(ctx, &store.UpdateAttachment{ID: attachment.ID, UpdatedTs: &updatedTs}))
	}

	access := &store.MemoAccessScope{UserID: &owner.ID}
	limit, firstOffset := 1, 0
	attachments, err := ts.ListAttachments(ctx, &store.FindAttachment{
		Access: access, Limit: &limit, Offset: &firstOffset,
	})
	require.NoError(t, err)
	require.Equal(t, []int32{owned.ID}, attachmentIDs(attachments), "check note ownership before pagination")
	secondOffset := 1
	attachments, err = ts.ListAttachments(ctx, &store.FindAttachment{
		Access: access, Limit: &limit, Offset: &secondOffset,
	})
	require.NoError(t, err)
	require.Equal(t, []int32{unlinked.ID}, attachmentIDs(attachments))
}

func TestAttachmentCreatorFilterUsesMemoAuthor(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	uploader, err := ts.CreateUser(ctx, &store.User{Username: "attachment-uploader", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	author, err := ts.CreateUser(ctx, &store.User{Username: "attachment-memo-author", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	memo, err := ts.CreateMemo(ctx, &store.Memo{
		UID: "attachment-creator-memo", CreatorID: author.ID, Content: "public memo", Visibility: store.Public,
	})
	require.NoError(t, err)
	linked, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: "attachment-creator-linked", CreatorID: uploader.ID, Filename: "linked.txt", Type: "text/plain", MemoID: &memo.ID,
	})
	require.NoError(t, err)
	unlinked, err := ts.CreateAttachment(ctx, &store.Attachment{
		UID: "attachment-creator-unlinked", CreatorID: uploader.ID, Filename: "unlinked.txt", Type: "text/plain",
	})
	require.NoError(t, err)

	list := func(filter string) []*store.Attachment {
		attachments, listErr := ts.ListAttachments(ctx, &store.FindAttachment{
			Filters: []string{filter}, SkipDefaultLimit: true,
		})
		require.NoError(t, listErr)
		return attachments
	}
	require.Equal(t, []int32{linked.ID}, attachmentIDs(list(`creator == "users/attachment-memo-author"`)))
	require.Equal(t, []int32{unlinked.ID}, attachmentIDs(list(`creator == "users/attachment-uploader"`)))
}

func attachmentIDs(attachments []*store.Attachment) []int32 {
	ids := make([]int32, 0, len(attachments))
	for _, attachment := range attachments {
		ids = append(ids, attachment.ID)
	}
	return ids
}
