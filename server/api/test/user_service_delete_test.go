package test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/store"
)

func TestDeleteUserSelfDeleteCleansAccountDataAndAuthCookies(t *testing.T) {
	t.Parallel()

	ts := NewTestService(t)
	defer ts.Cleanup()

	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "alice")
	require.NoError(t, err)

	_, err = ts.Store.GetDriver().GetDB().ExecContext(ctx, "INSERT INTO user_identity (user_id, provider, extern_uid) VALUES (?, ?, ?)", user.ID, "google", "alice-google-sub")
	require.NoError(t, err)

	err = ts.Store.AddUserRefreshToken(ctx, user.ID, &storepb.RefreshTokensUserSetting_RefreshToken{
		TokenId:   "refresh-token-id",
		ExpiresAt: timestamppb.New(time.Now().Add(time.Hour)),
		CreatedAt: timestamppb.Now(),
	})
	require.NoError(t, err)

	headerCtx := api.WithHeaderCarrier(ctx)
	authCtx := ts.CreateUserContext(headerCtx, user.ID)
	_, err = ts.Service.DeleteUser(authCtx, &apipb.DeleteUserRequest{
		Name: api.BuildUserName(user.Username),
	})
	require.NoError(t, err)

	deletedUser, err := ts.Store.GetUser(ctx, &store.FindUser{ID: &user.ID})
	require.NoError(t, err)
	require.Nil(t, deletedUser)

	var identities int
	require.NoError(t, ts.Store.GetDriver().GetDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM user_identity WHERE user_id = ?", user.ID).Scan(&identities))
	require.Zero(t, identities)

	refreshSetting, err := ts.Store.GetUserSetting(ctx, &store.FindUserSetting{
		UserID: &user.ID,
		Key:    storepb.UserSetting_REFRESH_TOKENS,
	})
	require.NoError(t, err)
	require.Nil(t, refreshSetting)

	carrier := api.GetHeaderCarrier(authCtx)
	require.NotNil(t, carrier)
	require.Contains(t, strings.ToLower(carrier.Get("Set-Cookie")), "memos_refresh=")
}

func TestDeleteUserSelfDeleteRemovesOnlyOwnedMemosAndResources(t *testing.T) {
	t.Parallel()

	ts := NewTestService(t)
	defer ts.Cleanup()

	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "resource-owner")
	require.NoError(t, err)
	peer, err := ts.CreateRegularUser(ctx, "resource-peer")
	require.NoError(t, err)

	// Seed legacy cross-author threads through the store to exercise deletion
	// of historical relationships without creating them via the personal-note API.
	createMemo := func(uid string, creatorID int32, content string) *store.Memo {
		t.Helper()
		memo, err := ts.Store.CreateMemo(ctx, &store.Memo{UID: uid, CreatorID: creatorID, Content: content, Visibility: store.Public})
		require.NoError(t, err)
		return memo
	}
	createLegacyComment := func(uid string, creatorID, parentID int32, content string) *store.Memo {
		t.Helper()
		comment := createMemo(uid, creatorID, content)
		_, err := ts.Store.GetDriver().GetDB().ExecContext(ctx, "INSERT INTO memo_relation (memo_id, related_memo_id, type) VALUES (?, ?, 'COMMENT')", comment.ID, parentID)
		require.NoError(t, err)
		return comment
	}
	ownMemoStore := createMemo("legacy-owner-memo", user.ID, "owner memo")
	foreignMemoStore := createMemo("legacy-peer-memo", peer.ID, "peer memo")
	peerCommentOnOwnMemo := createLegacyComment("legacy-peer-comment", peer.ID, ownMemoStore.ID, "peer comment on owner memo")
	peerNestedCommentOnOwnMemo := createLegacyComment("legacy-peer-nested", peer.ID, peerCommentOnOwnMemo.ID, "peer nested comment on owner memo")
	userCommentOnForeignMemo := createLegacyComment("legacy-owner-comment", user.ID, foreignMemoStore.ID, "owner comment on peer memo")
	peerReplyToUserComment := createLegacyComment("legacy-peer-reply", peer.ID, userCommentOnForeignMemo.ID, "peer reply to owner comment")

	attachedAttachment, err := ts.Store.CreateAttachment(ctx, &store.Attachment{
		UID:       "attach-owner-memo",
		CreatorID: user.ID,
		Filename:  "owner.txt",
		Type:      "text/plain",
		Size:      4,
		Blob:      []byte("memo"),
		MemoID:    &ownMemoStore.ID,
	})
	require.NoError(t, err)
	thumbnailCachePath := filepath.Join(ts.Profile.Data, ".thumbnail_cache", attachedAttachment.UID+".jpeg")
	motionCachePath := filepath.Join(ts.Profile.Data, ".motion_cache", attachedAttachment.UID+".mp4")
	require.NoError(t, os.MkdirAll(filepath.Dir(thumbnailCachePath), 0o755))
	require.NoError(t, os.WriteFile(thumbnailCachePath, []byte("thumb"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Dir(motionCachePath), 0o755))
	require.NoError(t, os.WriteFile(motionCachePath, []byte("motion"), 0o644))

	unattachedAttachment, err := ts.Store.CreateAttachment(ctx, &store.Attachment{
		UID:       "attach-owner-loose",
		CreatorID: user.ID,
		Filename:  "loose.txt",
		Type:      "text/plain",
		Size:      5,
		Blob:      []byte("loose"),
	})
	require.NoError(t, err)

	peerAttachment, err := ts.Store.CreateAttachment(ctx, &store.Attachment{
		UID:       "attach-peer-keep",
		CreatorID: peer.ID,
		Filename:  "peer.txt",
		Type:      "text/plain",
		Size:      4,
		Blob:      []byte("peer"),
		MemoID:    &foreignMemoStore.ID,
	})
	require.NoError(t, err)
	peerCtx := ts.CreateUserContext(ctx, peer.ID)
	crossLinkedAttachment, err := ts.Service.CreateAttachment(peerCtx, &apipb.CreateAttachmentRequest{
		AttachmentId: "attach-peer-detach",
		Attachment: &apipb.Attachment{
			Filename: "peer-old-thread.txt", Type: "text/plain", Content: []byte("peer keeps it"),
		},
	})
	require.NoError(t, err)
	crossLinkedUID, err := api.ExtractAttachmentUIDFromName(crossLinkedAttachment.Name)
	require.NoError(t, err)
	_, err = ts.Store.GetDriver().GetDB().ExecContext(ctx, "UPDATE attachment SET memo_id = ? WHERE uid = ?", ownMemoStore.ID, crossLinkedUID)
	require.NoError(t, err)

	_, err = ts.Store.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    peer.ID,
		MemoID:       ownMemoStore.ID,
		ReactionType: "👍",
	})
	require.NoError(t, err)
	_, err = ts.Store.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    peer.ID,
		MemoID:       userCommentOnForeignMemo.ID,
		ReactionType: "🔥",
	})
	require.NoError(t, err)
	_, err = ts.Store.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		MemoID:       foreignMemoStore.ID,
		ReactionType: "👋",
	})
	require.NoError(t, err)
	peerReactionOnForeignMemo, err := ts.Store.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    peer.ID,
		MemoID:       foreignMemoStore.ID,
		ReactionType: "✅",
	})
	require.NoError(t, err)

	_, err = ts.Store.CreateMemoShare(ctx, &store.MemoShare{
		UID:       "share-owner-ownmemo",
		MemoID:    ownMemoStore.ID,
		CreatorID: user.ID,
	})
	require.NoError(t, err)
	_, err = ts.Store.CreateMemoShare(ctx, &store.MemoShare{
		UID:       "share-owner-foreignmemo",
		MemoID:    foreignMemoStore.ID,
		CreatorID: user.ID,
	})
	require.NoError(t, err)
	peerShare, err := ts.Store.CreateMemoShare(ctx, &store.MemoShare{
		UID:       "share-peer-foreignmemo",
		MemoID:    foreignMemoStore.ID,
		CreatorID: peer.ID,
	})
	require.NoError(t, err)

	for _, notification := range []struct {
		senderID, receiverID, memoID, parentID int32
	}{
		{peer.ID, user.ID, peerCommentOnOwnMemo.ID, ownMemoStore.ID},
		{user.ID, peer.ID, userCommentOnForeignMemo.ID, foreignMemoStore.ID},
	} {
		_, err := ts.Store.CreateInbox(ctx, &store.Inbox{
			SenderID: notification.senderID, ReceiverID: notification.receiverID, Status: store.UNREAD,
			Message: &storepb.InboxMessage{
				Type: storepb.InboxMessage_MEMO_COMMENT,
				Payload: &storepb.InboxMessage_MemoComment{
					MemoComment: &storepb.InboxMessage_MemoCommentPayload{
						MemoId: notification.memoID, RelatedMemoId: notification.parentID,
					},
				},
			},
		})
		require.NoError(t, err)
	}

	_, err = ts.Store.GetDriver().GetDB().ExecContext(ctx, "INSERT INTO user_identity (user_id, provider, extern_uid) VALUES (?, ?, ?)", user.ID, "google", "resource-owner-google-sub")
	require.NoError(t, err)
	err = ts.Store.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-owner",
		TokenHash:   "pat-owner-hash",
		Description: "owner pat",
	})
	require.NoError(t, err)

	headerCtx := api.WithHeaderCarrier(ctx)
	authCtx := ts.CreateUserContext(headerCtx, user.ID)
	_, err = ts.Service.DeleteUser(authCtx, &apipb.DeleteUserRequest{
		Name: api.BuildUserName(user.Username),
	})
	require.NoError(t, err)

	deletedUser, err := ts.Store.GetUser(ctx, &store.FindUser{ID: &user.ID})
	require.NoError(t, err)
	require.Nil(t, deletedUser)

	for _, memoID := range []int32{ownMemoStore.ID, userCommentOnForeignMemo.ID} {
		memo, err := ts.Store.GetMemo(ctx, &store.FindMemo{ID: &memoID})
		require.NoError(t, err)
		require.Nil(t, memo, "deleted owned memo %d", memoID)
	}
	for _, memoID := range []int32{foreignMemoStore.ID, peerCommentOnOwnMemo.ID, peerNestedCommentOnOwnMemo.ID, peerReplyToUserComment.ID} {
		memo, err := ts.Store.GetMemo(ctx, &store.FindMemo{ID: &memoID})
		require.NoError(t, err)
		require.NotNil(t, memo, "preserved peer memo %d", memoID)
	}

	nestedRelations, err := ts.Store.ListMemoRelations(ctx, &store.FindMemoRelation{MemoID: &peerNestedCommentOnOwnMemo.ID})
	require.NoError(t, err)
	require.Len(t, nestedRelations, 1)
	require.Equal(t, peerCommentOnOwnMemo.ID, nestedRelations[0].RelatedMemoID)
	for _, memoID := range []int32{ownMemoStore.ID, userCommentOnForeignMemo.ID} {
		incidentRelations, listErr := ts.Store.ListMemoRelations(ctx, &store.FindMemoRelation{MemoIDList: []int32{memoID}})
		require.NoError(t, listErr)
		require.Empty(t, incidentRelations)
	}
	contextlessReplyRelations, err := ts.Store.ListMemoRelations(ctx, &store.FindMemoRelation{MemoID: &peerReplyToUserComment.ID})
	require.NoError(t, err)
	require.Empty(t, contextlessReplyRelations)

	for _, attachmentID := range []int32{attachedAttachment.ID, unattachedAttachment.ID} {
		attachment, getErr := ts.Store.GetAttachment(ctx, &store.FindAttachment{ID: &attachmentID})
		require.NoError(t, getErr)
		require.Nil(t, attachment)
	}
	peerAttachmentAfterDelete, err := ts.Store.GetAttachment(ctx, &store.FindAttachment{ID: &peerAttachment.ID})
	require.NoError(t, err)
	require.NotNil(t, peerAttachmentAfterDelete)
	detached, err := ts.Store.GetAttachment(ctx, &store.FindAttachment{UID: &crossLinkedUID})
	require.NoError(t, err)
	require.NotNil(t, detached)
	require.Equal(t, peer.ID, detached.CreatorID)
	require.Nil(t, detached.MemoID)
	blob, err := ts.Service.GetAttachmentBlob(ctx, detached)
	require.NoError(t, err)
	require.Equal(t, []byte("peer keeps it"), blob)
	_, err = os.Stat(thumbnailCachePath)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(motionCachePath)
	require.ErrorIs(t, err, os.ErrNotExist)

	ownMemoReactions, err := ts.Store.ListReactions(ctx, &store.FindReaction{MemoID: &ownMemoStore.ID})
	require.NoError(t, err)
	require.Empty(t, ownMemoReactions)

	userCommentReactions, err := ts.Store.ListReactions(ctx, &store.FindReaction{MemoID: &userCommentOnForeignMemo.ID})
	require.NoError(t, err)
	require.Empty(t, userCommentReactions)

	foreignMemoReactions, err := ts.Store.ListReactions(ctx, &store.FindReaction{MemoID: &foreignMemoStore.ID})
	require.NoError(t, err)
	require.Len(t, foreignMemoReactions, 1)
	require.Equal(t, peerReactionOnForeignMemo.ID, foreignMemoReactions[0].ID)

	ownerShares, err := ts.Store.ListMemoShares(ctx, &store.FindMemoShare{CreatorID: &user.ID})
	require.NoError(t, err)
	require.Empty(t, ownerShares)

	peerShares, err := ts.Store.ListMemoShares(ctx, &store.FindMemoShare{CreatorID: &peer.ID})
	require.NoError(t, err)
	require.Len(t, peerShares, 1)
	require.Equal(t, peerShare.ID, peerShares[0].ID)

	sentInboxes, err := ts.Store.ListInboxes(ctx, &store.FindInbox{SenderID: &user.ID})
	require.NoError(t, err)
	require.Empty(t, sentInboxes)
	receivedInboxes, err := ts.Store.ListInboxes(ctx, &store.FindInbox{ReceiverID: &user.ID})
	require.NoError(t, err)
	require.Empty(t, receivedInboxes)

	var identities int
	require.NoError(t, ts.Store.GetDriver().GetDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM user_identity WHERE user_id = ?", user.ID).Scan(&identities))
	require.Zero(t, identities)

	patSetting, err := ts.Store.GetUserSetting(ctx, &store.FindUserSetting{
		UserID: &user.ID,
		Key:    storepb.UserSetting_PERSONAL_ACCESS_TOKENS,
	})
	require.NoError(t, err)
	require.Nil(t, patSetting)
}

func TestArchiveAndDeleteAccountWithHistoricalSpaceMembership(t *testing.T) {
	ctx := t.Context()
	ts := NewTestService(t)
	defer ts.Cleanup()

	admin, err := ts.CreateHostUser(ctx, "instance-admin")
	require.NoError(t, err)
	user, err := ts.CreateRegularUser(ctx, "legacy-space-admin")
	require.NoError(t, err)
	adminCtx := ts.CreateUserContext(ctx, admin.ID)
	db := ts.Store.GetDriver().GetDB()
	var spaceID int32
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO space (uid, title) VALUES (?, ?) RETURNING id", "old-space", "Historical Space").Scan(&spaceID))
	_, err = db.ExecContext(ctx, "INSERT INTO space_member (space_id, user_id, status, role) VALUES (?, ?, 'ACTIVE', 'ADMIN'), (?, ?, 'ACTIVE', 'USER')", spaceID, user.ID, spaceID, admin.ID)
	require.NoError(t, err)

	userName := api.BuildUserName(user.Username)
	updated, err := ts.Service.UpdateUser(adminCtx, &apipb.UpdateUserRequest{
		User:       &apipb.User{Name: userName, State: apipb.State_ARCHIVED},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
	})
	require.NoError(t, err)
	require.Equal(t, apipb.State_ARCHIVED, updated.State)
	var memberships int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM space_member WHERE space_id = ?", spaceID).Scan(&memberships))
	require.Equal(t, 2, memberships, "archiving preserves historical metadata")

	_, err = ts.Service.DeleteUser(adminCtx, &apipb.DeleteUserRequest{Name: userName})
	require.NoError(t, err)
	deleted, err := ts.Store.GetUser(ctx, &store.FindUser{ID: &user.ID})
	require.NoError(t, err)
	require.Nil(t, deleted)

	var remainingUser int32
	require.NoError(t, db.QueryRowContext(ctx, "SELECT user_id FROM space_member WHERE space_id = ?", spaceID).Scan(&remainingUser))
	require.Equal(t, admin.ID, remainingUser)
	var title string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT title FROM space WHERE id = ?", spaceID).Scan(&title))
	require.Equal(t, "Historical Space", title)
}

func TestDeleteUserRollbackPreservesAllResources(t *testing.T) {
	t.Parallel()

	ts := NewTestService(t)
	defer ts.Cleanup()

	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "rollback-owner")
	require.NoError(t, err)
	peer, err := ts.CreateRegularUser(ctx, "rollback-peer")
	require.NoError(t, err)

	userCtx := ts.CreateUserContext(ctx, user.ID)
	ownMemo, err := ts.Service.CreateMemo(userCtx, &apipb.CreateMemoRequest{
		Memo: &apipb.Memo{
			Content:    "rollback owner memo",
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)

	ownMemoUID, err := api.ExtractMemoUIDFromName(ownMemo.Name)
	require.NoError(t, err)
	ownMemoStore, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &ownMemoUID})
	require.NoError(t, err)
	require.NotNil(t, ownMemoStore)

	attachment, err := ts.Store.CreateAttachment(ctx, &store.Attachment{
		UID:       "attach-rollback-owner",
		CreatorID: user.ID,
		Filename:  "rollback.txt",
		Type:      "text/plain",
		Size:      8,
		Blob:      []byte("rollback"),
		MemoID:    &ownMemoStore.ID,
	})
	require.NoError(t, err)

	reaction, err := ts.Store.UpsertReaction(ctx, &store.Reaction{
		CreatorID:    user.ID,
		MemoID:       ownMemoStore.ID,
		ReactionType: "💥",
	})
	require.NoError(t, err)

	share, err := ts.Store.CreateMemoShare(ctx, &store.MemoShare{
		UID:       "share-rollback-owner",
		MemoID:    ownMemoStore.ID,
		CreatorID: user.ID,
	})
	require.NoError(t, err)

	inbox, err := ts.Store.CreateInbox(ctx, &store.Inbox{
		SenderID:   peer.ID,
		ReceiverID: user.ID,
		Status:     store.UNREAD,
		Message: &storepb.InboxMessage{
			Type: storepb.InboxMessage_MEMO_COMMENT,
			Payload: &storepb.InboxMessage_MemoComment{
				MemoComment: &storepb.InboxMessage_MemoCommentPayload{
					MemoId: ownMemoStore.ID,
				},
			},
		},
	})
	require.NoError(t, err)

	_, err = ts.Store.GetDriver().GetDB().ExecContext(ctx, "INSERT INTO user_identity (user_id, provider, extern_uid) VALUES (?, ?, ?)", user.ID, "google", "rollback-owner-google-sub")
	require.NoError(t, err)
	err = ts.Store.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-rollback-owner",
		TokenHash:   "pat-rollback-owner-hash",
		Description: "rollback pat",
	})
	require.NoError(t, err)

	headerCtx := api.WithHeaderCarrier(ctx)
	failCtx := store.WithDeleteUserFailpoint(headerCtx, store.DeleteUserFailpointBeforeCommit)
	authCtx := ts.CreateUserContext(failCtx, user.ID)
	_, err = ts.Service.DeleteUser(authCtx, &apipb.DeleteUserRequest{
		Name: api.BuildUserName(user.Username),
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "delete user failpoint before commit")

	userAfterRollback, err := ts.Store.GetUser(ctx, &store.FindUser{ID: &user.ID})
	require.NoError(t, err)
	require.NotNil(t, userAfterRollback)

	memoAfterRollback, err := ts.Store.GetMemo(ctx, &store.FindMemo{UID: &ownMemoUID})
	require.NoError(t, err)
	require.NotNil(t, memoAfterRollback)

	attachmentAfterRollback, err := ts.Store.GetAttachment(ctx, &store.FindAttachment{ID: &attachment.ID})
	require.NoError(t, err)
	require.NotNil(t, attachmentAfterRollback)

	reactionAfterRollback, err := ts.Store.GetReaction(ctx, &store.FindReaction{ID: &reaction.ID})
	require.NoError(t, err)
	require.NotNil(t, reactionAfterRollback)

	shareAfterRollback, err := ts.Store.GetMemoShare(ctx, &store.FindMemoShare{ID: &share.ID})
	require.NoError(t, err)
	require.NotNil(t, shareAfterRollback)

	inboxesAfterRollback, err := ts.Store.ListInboxes(ctx, &store.FindInbox{ID: &inbox.ID})
	require.NoError(t, err)
	require.Len(t, inboxesAfterRollback, 1)

	var subject string
	require.NoError(t, ts.Store.GetDriver().GetDB().QueryRowContext(ctx, "SELECT extern_uid FROM user_identity WHERE user_id = ?", user.ID).Scan(&subject))
	require.Equal(t, "rollback-owner-google-sub", subject)

	patSetting, err := ts.Store.GetUserSetting(ctx, &store.FindUserSetting{
		UserID: &user.ID,
		Key:    storepb.UserSetting_PERSONAL_ACCESS_TOKENS,
	})
	require.NoError(t, err)
	require.NotNil(t, patSetting)
}

func TestDeleteUserReportsPostCommitAttachmentStorageCleanupFailure(t *testing.T) {
	t.Parallel()

	ts := NewTestService(t)
	defer ts.Cleanup()

	ctx := context.Background()
	user, err := ts.CreateRegularUser(ctx, "cleanup-failure-owner")
	require.NoError(t, err)

	_, err = ts.Store.CreateAttachment(ctx, &store.Attachment{
		UID:         "attach-cleanup-failure",
		CreatorID:   user.ID,
		Filename:    "failure.txt",
		Type:        "text/plain",
		Size:        7,
		Blob:        []byte("failure"),
		StorageType: storepb.AttachmentStorageType_LOCAL,
		Reference:   "cleanup-failure.txt",
	})
	require.NoError(t, err)
	attachmentPath := filepath.Join(ts.Profile.Data, "cleanup-failure.txt")
	require.NoError(t, os.WriteFile(attachmentPath, []byte("failure"), 0o600))

	headerCtx := api.WithHeaderCarrier(ctx)
	authCtx := ts.CreateUserContext(store.WithDeleteAttachmentStorageFailpoint(headerCtx), user.ID)
	_, err = ts.Service.DeleteUser(authCtx, &apipb.DeleteUserRequest{
		Name: api.BuildUserName(user.Username),
	})
	require.Equal(t, codes.Internal, status.Code(err))
	require.ErrorContains(t, err, "user was deleted but attachment storage cleanup failed")
	require.ErrorContains(t, err, store.ErrDeleteAttachmentStorageFailpoint.Error())

	deletedUser, err := ts.Store.GetUser(ctx, &store.FindUser{ID: &user.ID})
	require.NoError(t, err)
	require.Nil(t, deletedUser)
	require.FileExists(t, attachmentPath, "storage cleanup failure happens after the database deletion commits")
}
