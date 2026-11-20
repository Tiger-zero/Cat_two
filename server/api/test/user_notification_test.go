package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/usememos/memos/internal/email"
	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

func seedLegacyCommentInbox(t *testing.T, ts *TestService, owner, sender *store.User) (string, string) {
	t.Helper()
	ctx := context.Background()
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	parent, err := ts.Service.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{
		Memo: &apipb.Memo{Content: "Base memo"},
	})
	require.NoError(t, err)
	comment, err := ts.Service.CreateMemoComment(ownerCtx, &apipb.CreateMemoCommentRequest{
		Parent:  parent.Name,
		Comment: &apipb.Memo{Content: "Comment content"},
	})
	require.NoError(t, err)
	_, err = ts.Store.CreateInbox(ctx, &store.Inbox{
		SenderID:   sender.ID,
		ReceiverID: owner.ID,
		Status:     store.UNREAD,
		Message: &storepb.InboxMessage{
			Type: storepb.InboxMessage_MEMO_COMMENT,
			Payload: &storepb.InboxMessage_MemoComment{
				MemoComment: &storepb.InboxMessage_MemoCommentPayload{
					MemoId:        parseMemoIDFromNameForTest(t, ts, comment.Name),
					RelatedMemoId: parseMemoIDFromNameForTest(t, ts, parent.Name),
				},
			},
		},
	})
	require.NoError(t, err)
	return parent.Name, comment.Name
}

func TestListUserNotificationsIncludesReadableLegacyMemoCommentPayload(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "notification-owner")
	require.NoError(t, err)
	sender, err := ts.CreateRegularUser(ctx, "notification-sender")
	require.NoError(t, err)
	parentName, commentName := seedLegacyCommentInbox(t, ts, owner, sender)

	resp, err := ts.Service.ListUserNotifications(ts.CreateUserContext(ctx, owner.ID), &apipb.ListUserNotificationsRequest{
		Parent: "users/" + owner.Username,
	})
	require.NoError(t, err)
	require.Len(t, resp.Notifications, 1)

	notification := resp.Notifications[0]
	require.Contains(t, notification.Name, "users/"+owner.Username+"/notifications/")
	require.Equal(t, "users/"+sender.Username, notification.Sender)
	require.Equal(t, sender.Username, notification.GetSenderUser().GetUsername())
	require.Equal(t, apipb.UserNotification_MEMO_COMMENT, notification.Type)
	require.Equal(t, commentName, notification.GetMemoComment().GetMemo())
	require.Equal(t, parentName, notification.GetMemoComment().GetRelatedMemo())
	require.Equal(t, "Comment content", notification.GetMemoComment().GetMemoSnippet())
	require.Equal(t, "Base memo", notification.GetMemoComment().GetRelatedMemoSnippet())
}

func TestListUserNotificationsHidesLegacyNotificationsWithUnreadableMemos(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	owner, err := ts.CreateRegularUser(ctx, "notification-owner")
	require.NoError(t, err)
	sender, err := ts.CreateRegularUser(ctx, "notification-sender")
	require.NoError(t, err)
	foreignMemo, err := ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "legacy-notification-memo",
		CreatorID:  sender.ID,
		Content:    "legacy public memo",
		Visibility: store.Public,
	})
	require.NoError(t, err)

	_, err = ts.Store.CreateInbox(ctx, &store.Inbox{
		SenderID:   sender.ID,
		ReceiverID: owner.ID,
		Status:     store.UNREAD,
		Message: &storepb.InboxMessage{
			Type: storepb.InboxMessage_MEMO_MENTION,
			Payload: &storepb.InboxMessage_MemoMention{
				MemoMention: &storepb.InboxMessage_MemoMentionPayload{MemoId: foreignMemo.ID},
			},
		},
	})
	require.NoError(t, err)

	resp, err := ts.Service.ListUserNotifications(ts.CreateUserContext(ctx, owner.ID), &apipb.ListUserNotificationsRequest{
		Parent: "users/" + owner.Username,
	})
	require.NoError(t, err)
	require.Empty(t, resp.Notifications)
}

func TestPrivateMemoDoesNotSendCrossUserNotifications(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	var sentMessage *email.Message
	ts.Service.NotificationEmailSender = func(_ *email.Config, message *email.Message) {
		sentMessage = message
	}
	_, err := ts.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_NOTIFICATION,
		Value: &storepb.InstanceSetting_NotificationSetting{
			NotificationSetting: &storepb.InstanceNotificationSetting{
				Email: &storepb.InstanceNotificationSetting_EmailSetting{
					Enabled:   true,
					SmtpHost:  "smtp.example.com",
					SmtpPort:  587,
					FromEmail: "bot@example.com",
				},
			},
		},
	})
	require.NoError(t, err)

	owner, err := ts.CreateRegularUser(ctx, "notification-owner")
	require.NoError(t, err)
	otherUser, err := ts.CreateRegularUser(ctx, "notification-other")
	require.NoError(t, err)
	newTarget, err := ts.CreateRegularUser(ctx, "notification-new-target")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	otherCtx := ts.CreateUserContext(ctx, otherUser.ID)

	memo, err := ts.Service.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{
		Memo: &apipb.Memo{
			Content:    "Hello @" + otherUser.Username,
			Visibility: apipb.Visibility_PUBLIC,
		},
	})
	require.NoError(t, err)
	require.Equal(t, apipb.Visibility_PRIVATE, memo.Visibility)

	_, err = ts.Service.CreateMemoComment(otherCtx, &apipb.CreateMemoCommentRequest{
		Parent:  memo.Name,
		Comment: &apipb.Memo{Content: "Comment for @" + owner.Username},
	})
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	_, err = ts.Service.UpdateMemo(ownerCtx, &apipb.UpdateMemoRequest{
		Memo:       &apipb.Memo{Name: memo.Name, Content: "Hello @" + otherUser.Username + " and @" + newTarget.Username},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}},
	})
	require.NoError(t, err)
	require.Nil(t, sentMessage)

	for _, receiver := range []*store.User{owner, otherUser, newTarget} {
		inboxes, err := ts.Store.ListInboxes(ctx, &store.FindInbox{ReceiverID: &receiver.ID})
		require.NoError(t, err)
		require.Empty(t, inboxes)
	}
}

func TestListUserNotificationsOmitsNotificationWhenMemoDeleted(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "notification-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	sender, err := ts.CreateRegularUser(ctx, "notification-sender")
	require.NoError(t, err)
	parentName, _ := seedLegacyCommentInbox(t, ts, owner, sender)

	_, err = ts.Service.DeleteMemo(ownerCtx, &apipb.DeleteMemoRequest{Name: parentName})
	require.NoError(t, err)

	resp, err := ts.Service.ListUserNotifications(ownerCtx, &apipb.ListUserNotificationsRequest{Parent: "users/" + owner.Username})
	require.NoError(t, err)
	require.Empty(t, resp.Notifications)
}

func TestListUserNotificationsSkipsNotificationsWithMissingUsers(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "notification-owner")
	require.NoError(t, err)
	sender, err := ts.CreateRegularUser(ctx, "notification-sender")
	require.NoError(t, err)
	seedLegacyCommentInbox(t, ts, owner, sender)

	_, err = ts.Store.DeleteUser(ctx, &store.DeleteUser{ID: sender.ID})
	require.NoError(t, err)

	resp, err := ts.Service.ListUserNotifications(ts.CreateUserContext(ctx, owner.ID), &apipb.ListUserNotificationsRequest{
		Parent: "users/" + owner.Username,
	})
	require.NoError(t, err)
	require.Empty(t, resp.Notifications)
}

func TestUpdateUserNotificationRequiresUpdateMask(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "notification-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	sender, err := ts.CreateRegularUser(ctx, "notification-sender")
	require.NoError(t, err)
	seedLegacyCommentInbox(t, ts, owner, sender)

	resp, err := ts.Service.ListUserNotifications(ownerCtx, &apipb.ListUserNotificationsRequest{
		Parent: "users/" + owner.Username,
	})
	require.NoError(t, err)
	require.Len(t, resp.Notifications, 1)

	for _, mask := range []*fieldmaskpb.FieldMask{nil, {}} {
		_, err = ts.Service.UpdateUserNotification(ownerCtx, &apipb.UpdateUserNotificationRequest{
			Notification: &apipb.UserNotification{
				Name:   resp.Notifications[0].Name,
				Status: apipb.UserNotification_ARCHIVED,
			},
			UpdateMask: mask,
		})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	}
}

func TestListUserNotificationsRejectsNumericParent(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "notification-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	_, err = ts.Service.ListUserNotifications(ownerCtx, &apipb.ListUserNotificationsRequest{Parent: "users/1"})
	require.ErrorContains(t, err, "user not found")
}
