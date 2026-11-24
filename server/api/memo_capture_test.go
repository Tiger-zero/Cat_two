package api

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/store"
)

func newCaptureTestService(t *testing.T) *APIService {
	t.Helper()
	service := newIntegrationService(t)
	service.Profile.Personal = true
	service.Profile.Demo = false
	return service
}

func TestCaptureMemoKeepsSourceAndDeduplicatesTrashedMemo(t *testing.T) {
	ctx := t.Context()
	service := newCaptureTestService(t)
	owner := createSpaceTestUser(ctx, t, service, "capture-owner", store.RoleAdmin)
	client := service.SSEHub.Subscribe()
	defer service.SSEHub.Unsubscribe(client)

	const deliveryID = "same-delivery-id"
	const telegramText = "  原始正文\n\n```text\n#ignore-inside-unclosed-fence"
	require.NoError(t, service.CaptureMemo(ctx, "telegram", deliveryID, telegramText))
	requireMemoChanged(t, client.events)
	require.NoError(t, service.CaptureMemo(ctx, "wechat", deliveryID, "微信正文"))
	requireMemoChanged(t, client.events)

	telegramUID := capturedMemoUID("telegram", deliveryID)
	wechatUID := capturedMemoUID("wechat", deliveryID)
	require.NotEqual(t, telegramUID, wechatUID)
	for _, test := range []struct {
		uid, channel, tag, text string
	}{
		{telegramUID, "telegram", "Telegram", telegramText},
		{wechatUID, "wechat", "微信", "微信正文"},
	} {
		memo, err := service.Store.GetMemo(ctx, &store.FindMemo{UID: &test.uid})
		require.NoError(t, err)
		require.NotNil(t, memo)
		require.Equal(t, owner.ID, memo.CreatorID)
		require.Equal(t, store.Private, memo.Visibility)
		require.Nil(t, memo.ParentUID)
		require.Equal(t, "#"+test.tag+"\n\n"+test.text, memo.Content)
		require.Equal(t, test.channel, memo.Payload.GetCreationChannel())
		require.Equal(t, []string{test.tag}, memo.Payload.GetTags())
	}

	edited, err := service.UpdateMemo(userCtx(ctx, owner.ID), &apipb.UpdateMemoRequest{
		Memo:       &apipb.Memo{Name: "memos/" + telegramUID, Content: "edited after capture"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}},
	})
	require.NoError(t, err)
	requireMemoChanged(t, client.events)
	require.NoError(t, service.CaptureMemo(ctx, "telegram", deliveryID, telegramText))
	requireNoMemoChanged(t, client.events)
	_, err = service.DeleteMemo(userCtx(ctx, owner.ID), &apipb.DeleteMemoRequest{Name: edited.Name})
	require.NoError(t, err)
	requireMemoChanged(t, client.events)
	require.NoError(t, service.CaptureMemo(ctx, "telegram", deliveryID, telegramText))
	requireNoMemoChanged(t, client.events)

	trashed, err := service.Store.GetMemo(ctx, &store.FindMemo{UID: &telegramUID, Trash: store.MemoTrashInclude})
	require.NoError(t, err)
	require.NotNil(t, trashed)
	require.NotNil(t, trashed.DeletedTs)
	require.Equal(t, "edited after capture", trashed.Content)
	require.Equal(t, "telegram", trashed.Payload.GetCreationChannel())
}

func TestCaptureMemoDeduplicatesConcurrentRetries(t *testing.T) {
	ctx := t.Context()
	service := newCaptureTestService(t)
	createSpaceTestUser(ctx, t, service, "capture-concurrent-owner", store.RoleAdmin)
	client := service.SSEHub.Subscribe()
	defer service.SSEHub.Unsubscribe(client)

	const attempts = 8
	start := make(chan struct{})
	results := make(chan error, attempts)
	for range attempts {
		go func() {
			<-start
			results <- service.CaptureMemo(ctx, "telegram", "parallel-capture", "original")
		}()
	}
	close(start)
	for range attempts {
		assert.NoError(t, <-results)
	}
	requireMemoChanged(t, client.events)
	requireNoMemoChanged(t, client.events)
	memos, err := service.Store.ListMemos(ctx, &store.FindMemo{Trash: store.MemoTrashInclude})
	require.NoError(t, err)
	require.Len(t, memos, 1)
}

func TestCaptureMemoPreservesUnsupportedAttachmentMarkupAsText(t *testing.T) {
	ctx := t.Context()
	service := newCaptureTestService(t)
	createSpaceTestUser(ctx, t, service, "capture-markdown-owner", store.RoleAdmin)

	for _, test := range []struct {
		name, text, savedBody string
	}{
		{"normal-link", " [链接](https://example.com) ", " [链接](https://example.com) "},
		{"unbound-image", "正文\n![image](/file/attachments/missing)", "    正文\n    ![image](/file/attachments/missing)"},
		{"invalid-image", "![image](/file/attachments/bad_uid)\r\n尾部", "    ![image](/file/attachments/bad_uid)\r\n    尾部"},
		{"absolute-image", "![image](https://example.com/file/attachments/missing)", "    ![image](https://example.com/file/attachments/missing)"},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, service.CaptureMemo(ctx, "telegram", test.name, test.text))
			uid := capturedMemoUID("telegram", test.name)
			memo, err := service.Store.GetMemo(ctx, &store.FindMemo{UID: &uid})
			require.NoError(t, err)
			require.NotNil(t, memo)
			require.Equal(t, "#Telegram\n\n"+test.savedBody, memo.Content)
			require.Equal(t, []string{"Telegram"}, memo.Payload.GetTags())
		})
	}
}

func TestCaptureMemoRequiresOneActiveAdministrator(t *testing.T) {
	ctx := t.Context()
	service := newCaptureTestService(t)
	createSpaceTestUser(ctx, t, service, "capture-legacy-user", store.RoleUser)
	capture := func() error {
		return service.CaptureMemo(ctx, "wechat", "owner-selection", "private text")
	}
	require.Equal(t, codes.FailedPrecondition, status.Code(capture()))

	owner := createSpaceTestUser(ctx, t, service, "capture-single-owner", store.RoleAdmin)
	second := createSpaceTestUser(ctx, t, service, "capture-extra-admin", store.RoleAdmin)
	require.Equal(t, codes.FailedPrecondition, status.Code(capture()))
	_, err := service.Store.UpdateUser(ctx, &store.UpdateUser{ID: second.ID, RowStatus: new(store.Archived)})
	require.NoError(t, err)
	require.NoError(t, capture())
	memos, err := service.Store.ListMemos(ctx, &store.FindMemo{Trash: store.MemoTrashInclude})
	require.NoError(t, err)
	require.Len(t, memos, 1)
	require.Equal(t, owner.ID, memos[0].CreatorID)
}

func TestCaptureMemoRejectsDisabledOrInvalidInput(t *testing.T) {
	ctx := t.Context()
	service := newCaptureTestService(t)
	createSpaceTestUser(ctx, t, service, "capture-invalid-owner", store.RoleAdmin)
	client := service.SSEHub.Subscribe()
	defer service.SSEHub.Unsubscribe(client)
	service.Profile.Personal = false
	require.Equal(t, codes.FailedPrecondition, status.Code(service.CaptureMemo(ctx, "telegram", "disabled", "text")))
	service.Profile.Personal = true
	service.Profile.Demo = true
	require.Equal(t, codes.FailedPrecondition, status.Code(service.CaptureMemo(ctx, "wechat", "demo", "text")))
	service.Profile.Demo = false

	for _, test := range []struct {
		name, channel, id, text string
	}{
		{"unknown channel", "web", "42", "text"},
		{"blank ID", "telegram", " \t", "text"},
		{"oversize ID", "wechat", strings.Repeat("x", maxCaptureIDBytes+1), "text"},
		{"blank text", "wechat", "42", "\n\u3000"},
		{"oversize text", "telegram", "42", strings.Repeat("x", maxCaptureTextBytes+1)},
		{"invalid UTF-8", "wechat", "42", "\xff"},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, codes.InvalidArgument, status.Code(service.CaptureMemo(ctx, test.channel, test.id, test.text)))
		})
	}
	requireNoMemoChanged(t, client.events)
	memos, err := service.Store.ListMemos(ctx, &store.FindMemo{Trash: store.MemoTrashInclude})
	require.NoError(t, err)
	require.Empty(t, memos)
}
