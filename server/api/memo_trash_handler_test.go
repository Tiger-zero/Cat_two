package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/version"
	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/server/fileserver"
	"github.com/usememos/memos/store"
	"github.com/usememos/memos/store/db"
)

func runMemoTrashRequest(t *testing.T, svc *APIService, user *store.User, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	handler := svc.NewHandler()
	return runMemoTrashHTTP(t, handler, svc.Secret, user, method, target)
}

func runMemoTrashHTTP(t *testing.T, handler http.Handler, secret string, user *store.User, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	return runMemoTrashHTTPWithContext(t.Context(), t, handler, secret, user, method, target)
}

func runMemoTrashHTTPWithContext(ctx context.Context, t *testing.T, handler http.Handler, secret string, user *store.User, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(ctx, method, target, nil)
	if user != nil {
		token, _, err := auth.GenerateAccessTokenV2(user.ID, user.Username, string(user.Role), string(user.RowStatus), user.PasswordHash, []byte(secret))
		require.NoError(t, err)
		request.Header.Set("Authorization", "Bearer "+token)
	}
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)
	return result
}

func TestMemoTrashAttachmentCleanupRetriesAfterReopen(t *testing.T) {
	ctx := t.Context()
	p := &profile.Profile{Demo: true, Driver: "sqlite", Data: t.TempDir(), Version: version.GetCurrentVersion()}
	p.DSN = filepath.Join(p.Data, "memos.db")
	open := func() *APIService {
		t.Helper()
		driver, err := db.NewDBDriver(p)
		require.NoError(t, err)
		st := store.New(driver, p)
		t.Cleanup(func() { _ = st.Close() })
		require.NoError(t, st.InitializeSchema(ctx))
		return NewAPIService("test-secret", p, st)
	}

	svc := open()
	owner := createSpaceTestUser(ctx, t, svc, "cleanup-retry-owner", store.RoleUser)
	ownerCtx := userCtx(ctx, owner.ID)
	memo, err := svc.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{Memo: &apipb.Memo{Content: "memo to purge"}})
	require.NoError(t, err)
	createAttachment := func(filename, content string, memoName *string) *store.Attachment {
		t.Helper()
		attachment, err := svc.CreateAttachment(ownerCtx, &apipb.CreateAttachmentRequest{
			Attachment: &apipb.Attachment{Filename: filename, Type: "text/plain", Content: []byte(content), Memo: memoName},
		})
		require.NoError(t, err)
		uid, err := ExtractAttachmentUIDFromName(attachment.Name)
		require.NoError(t, err)
		stored, err := svc.Store.GetAttachment(ctx, &store.FindAttachment{UID: &uid})
		require.NoError(t, err)
		require.NotNil(t, stored)
		return stored
	}
	removed := createAttachment("retry.txt", "remove after restart", &memo.Name)
	reused := createAttachment("reused.txt", "old data", &memo.Name)
	_, err = svc.DeleteMemo(ownerCtx, &apipb.DeleteMemoRequest{Name: memo.Name})
	require.NoError(t, err)
	uid, err := ExtractMemoUIDFromName(memo.Name)
	require.NoError(t, err)
	handler := svc.NewHandler()
	response := runMemoTrashHTTPWithContext(store.WithDeleteAttachmentStorageFailpoint(ctx), t, handler, svc.Secret, owner, http.MethodDelete, "/internal/trash/"+uid)
	require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
	absent, err := svc.Store.GetMemo(ctx, &store.FindMemo{UID: &uid, Trash: store.MemoTrashInclude})
	require.NoError(t, err)
	require.Nil(t, absent)
	removedPath := filepath.Join(p.Data, filepath.FromSlash(removed.Reference))
	reusedPath := filepath.Join(p.Data, filepath.FromSlash(reused.Reference))
	require.FileExists(t, removedPath)

	// Simulate a process stopping after unlink but before acknowledging that
	// job. A new attachment may use the same filename while the job survives.
	require.NoError(t, os.Remove(reusedPath))
	replacement := createAttachment("reused.txt", "replacement data", nil)
	require.NotEqual(t, reused.Reference, replacement.Reference)
	require.NoError(t, svc.Store.Close())
	svc = open()
	require.NoError(t, svc.PurgeExpiredMemoTrash(ctx, time.Now().Add(time.Hour)))
	require.NoFileExists(t, removedPath)
	data, err := svc.GetAttachmentBlob(ctx, replacement)
	require.NoError(t, err)
	require.Equal(t, "replacement data", string(data))
	pending, err := svc.Store.ListMemoTrashAttachmentCleanup(ctx, 0, time.Now().Add(24*time.Hour), 100)
	require.NoError(t, err)
	require.Empty(t, pending)
}

func TestMemoTrashExpiryKeepsSharedLocalAttachment(t *testing.T) {
	ctx := t.Context()
	svc := newIntegrationService(t)
	owner := createSpaceTestUser(ctx, t, svc, "cleanup-existing-owner", store.RoleUser)
	memo, err := svc.Store.CreateMemo(ctx, &store.Memo{UID: "expired-attachment-note", CreatorID: owner.ID, Content: "removed", Visibility: store.Private})
	require.NoError(t, err)
	keepPath := filepath.Join(svc.Profile.Data, "shared.txt")
	removePath := filepath.Join(svc.Profile.Data, "old.txt")
	require.NoError(t, os.WriteFile(keepPath, []byte("keep"), 0o600))
	require.NoError(t, os.WriteFile(removePath, []byte("remove"), 0o600))
	for _, attachment := range []*store.Attachment{
		{UID: "trashed-shared-file", CreatorID: owner.ID, MemoID: &memo.ID, StorageType: storepb.AttachmentStorageType_LOCAL, Reference: "shared.txt"},
		{UID: "trashed-old-file", CreatorID: owner.ID, MemoID: &memo.ID, StorageType: storepb.AttachmentStorageType_LOCAL, Reference: "old.txt"},
		{UID: "active-shared-file", CreatorID: owner.ID, StorageType: storepb.AttachmentStorageType_LOCAL, Reference: filepath.ToSlash(svc.Profile.Data) + "/./shared.txt"},
	} {
		_, err := svc.Store.CreateAttachment(ctx, attachment)
		require.NoError(t, err)
	}
	now := time.Now()
	_, err = svc.Store.ApplyMemoTrash(ctx, &store.MemoTrashMutation{
		MemoID: memo.ID, ActorUserID: owner.ID, Action: store.MemoTrashMove, NowTs: now.Add(-store.MemoTrashRetention).Unix(),
	})
	require.NoError(t, err)
	require.NoError(t, svc.PurgeExpiredMemoTrash(store.WithDeleteAttachmentStorageFailpoint(ctx), now))
	require.FileExists(t, keepPath)
	require.FileExists(t, removePath)
	require.NoError(t, svc.PurgeExpiredMemoTrash(ctx, now.Add(time.Hour)))
	require.FileExists(t, keepPath)
	require.NoFileExists(t, removePath)
}

func TestMemoTrashCleanupDoesNotQueueRetiredObjectStorage(t *testing.T) {
	ctx := t.Context()
	svc := newIntegrationService(t)
	owner := createSpaceTestUser(ctx, t, svc, "cleanup-s3-owner", store.RoleUser)
	memo, err := svc.Store.CreateMemo(ctx, &store.Memo{UID: "trashed-s3-memo", CreatorID: owner.ID, Content: "removed", Visibility: store.Private})
	require.NoError(t, err)
	attachment, err := svc.Store.CreateAttachment(ctx, &store.Attachment{
		UID: "retired-object", CreatorID: owner.ID, MemoID: &memo.ID, StorageType: storepb.AttachmentStorageType_S3,
		Payload: &storepb.AttachmentPayload{Payload: &storepb.AttachmentPayload_S3Object_{
			S3Object: &storepb.AttachmentPayload_S3Object{Key: "legacy/original.txt"},
		}},
	})
	require.NoError(t, err)

	now := time.Now()
	_, err = svc.Store.ApplyMemoTrash(ctx, &store.MemoTrashMutation{
		MemoID: memo.ID, ActorUserID: owner.ID, Action: store.MemoTrashMove, NowTs: now.Add(-store.MemoTrashRetention).Unix(),
	})
	require.NoError(t, err)
	require.NoError(t, svc.PurgeExpiredMemoTrash(ctx, now))
	stored, err := svc.Store.GetAttachment(ctx, &store.FindAttachment{ID: &attachment.ID})
	require.NoError(t, err)
	require.Nil(t, stored)
	pending, err := svc.Store.ListMemoTrashAttachmentCleanup(ctx, 0, now.Add(24*time.Hour), 100)
	require.NoError(t, err)
	require.Empty(t, pending)
}

func TestMemoTrashBrowserAndRestore(t *testing.T) {
	ctx := t.Context()
	svc := newIntegrationService(t)
	owner := createSpaceTestUser(ctx, t, svc, "trashed-note-owner", store.RoleUser)
	admin := createSpaceTestUser(ctx, t, svc, "unrelated-trash-admin", store.RoleAdmin)
	ownerCtx := userCtx(ctx, owner.ID)
	memo, err := svc.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{Memo: &apipb.Memo{Content: "kept **content**"}})
	require.NoError(t, err)
	reply, err := svc.CreateMemoComment(ownerCtx, &apipb.CreateMemoCommentRequest{Parent: memo.Name, Comment: &apipb.Memo{Content: "kept reply"}})
	require.NoError(t, err)
	attachment, err := svc.CreateAttachment(ownerCtx, &apipb.CreateAttachmentRequest{Attachment: &apipb.Attachment{
		Filename: "kept.txt", Type: "text/plain", Content: []byte("kept attachment"), Memo: &memo.Name,
	}})
	require.NoError(t, err)
	_, err = svc.UpdateMemo(ownerCtx, &apipb.UpdateMemoRequest{
		Memo: &apipb.Memo{Name: memo.Name, State: apipb.State_ARCHIVED, Pinned: true}, UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state", "pinned"}},
	})
	require.NoError(t, err)
	_, err = svc.DeleteMemo(ownerCtx, &apipb.DeleteMemoRequest{Name: memo.Name, Force: true})
	require.NoError(t, err)
	_, err = svc.GetMemo(ownerCtx, &apipb.GetMemoRequest{Name: memo.Name})
	require.Equal(t, codes.NotFound, status.Code(err))
	_, err = svc.GetMemo(ownerCtx, &apipb.GetMemoRequest{Name: reply.Name})
	require.Equal(t, codes.NotFound, status.Code(err))
	_, err = svc.GetAttachment(ownerCtx, &apipb.GetAttachmentRequest{Name: attachment.Name})
	require.Equal(t, codes.NotFound, status.Code(err))
	fileMux := http.NewServeMux()
	fileserver.NewFileServerService(svc.Profile, svc.Store, svc.Secret).RegisterRoutes(fileMux, http.NotFoundHandler())
	readFile := func() *httptest.ResponseRecorder {
		return runMemoTrashHTTP(t, fileMux, svc.Secret, owner, http.MethodGet, "/file/"+attachment.Name)
	}
	require.Equal(t, http.StatusNotFound, readFile().Code)
	archived, err := svc.ListMemos(ownerCtx, &apipb.ListMemosRequest{State: apipb.State_ARCHIVED})
	require.NoError(t, err)
	require.Empty(t, archived.Memos)

	uid, err := ExtractMemoUIDFromName(memo.Name)
	require.NoError(t, err)
	path := "/internal/trash/" + uid
	require.Equal(t, http.StatusUnauthorized, runMemoTrashRequest(t, svc, nil, http.MethodGet, "/internal/trash").Code)
	require.Equal(t, http.StatusNotFound, runMemoTrashRequest(t, svc, admin, http.MethodPost, path+"/restore").Code)
	require.Equal(t, http.StatusNotFound, runMemoTrashRequest(t, svc, admin, http.MethodDelete, path).Code)
	adminList := runMemoTrashRequest(t, svc, admin, http.MethodGet, "/internal/trash")
	require.Equal(t, http.StatusOK, adminList.Code)
	require.JSONEq(t, `{"memos":[],"nextPageToken":""}`, adminList.Body.String())
	request := runMemoTrashRequest(t, svc, owner, http.MethodGet, "/internal/trash")
	require.Equal(t, http.StatusOK, request.Code)
	require.Contains(t, request.Header().Get("Cache-Control"), "no-store")
	var page memoTrashPage
	require.NoError(t, json.Unmarshal(request.Body.Bytes(), &page))
	require.Len(t, page.Memos, 1)
	require.WithinDuration(t, time.Now(), page.Memos[0].DeletedTime, 5*time.Second)
	var deletedMemo apipb.Memo
	require.NoError(t, protojson.Unmarshal(page.Memos[0].Memo, &deletedMemo))
	require.Equal(t, memo.Name, deletedMemo.Name)
	require.Equal(t, memo.Content, deletedMemo.Content)
	require.Equal(t, apipb.State_ARCHIVED, deletedMemo.State)
	require.Len(t, deletedMemo.Attachments, 1)
	require.Equal(t, attachment.Filename, deletedMemo.Attachments[0].Filename)
	response := runMemoTrashRequest(t, svc, owner, http.MethodPost, path+"/restore")
	require.Equal(t, http.StatusNoContent, response.Code, response.Body.String())
	restored, err := svc.GetMemo(ownerCtx, &apipb.GetMemoRequest{Name: memo.Name})
	require.NoError(t, err)
	require.Equal(t, apipb.State_ARCHIVED, restored.State)
	require.True(t, restored.Pinned)
	_, err = svc.GetMemo(ownerCtx, &apipb.GetMemoRequest{Name: reply.Name})
	require.NoError(t, err)
	_, err = svc.GetAttachment(ownerCtx, &apipb.GetAttachmentRequest{Name: attachment.Name})
	require.NoError(t, err)
	file := readFile()
	require.Equal(t, http.StatusOK, file.Code)
	require.Equal(t, "kept attachment", file.Body.String())
}

func TestMemoTrashPaginationAndExpiry(t *testing.T) {
	ctx := t.Context()
	svc := newIntegrationService(t)
	owner := createSpaceTestUser(ctx, t, svc, "trash-expiry-owner", store.RoleUser)
	now := time.Now().Truncate(time.Second)
	trashMemo := func(id string, deletedTime time.Time) *store.Memo {
		t.Helper()
		memo, err := svc.Store.CreateMemo(ctx, &store.Memo{UID: id, CreatorID: owner.ID, Content: id, Visibility: store.Private})
		require.NoError(t, err)
		_, err = svc.Store.ApplyMemoTrash(ctx, &store.MemoTrashMutation{MemoID: memo.ID, ActorUserID: owner.ID, Action: store.MemoTrashMove, NowTs: deletedTime.Unix()})
		require.NoError(t, err)
		return memo
	}
	expired := trashMemo("expiredmemo", now.Add(-store.MemoTrashRetention))
	older := trashMemo("trashedolder", now.Add(-24*time.Hour))
	newer := trashMemo("trashednewer", now)
	first := runMemoTrashRequest(t, svc, owner, http.MethodGet, "/internal/trash?pageSize=1")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var page memoTrashPage
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &page))
	require.Len(t, page.Memos, 1)
	require.Contains(t, string(page.Memos[0].Memo), newer.UID)
	require.NotEmpty(t, page.NextPageToken)
	second := runMemoTrashRequest(t, svc, owner, http.MethodGet, "/internal/trash?pageToken="+url.QueryEscape(page.NextPageToken))
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &page))
	require.Len(t, page.Memos, 1)
	require.Contains(t, string(page.Memos[0].Memo), older.UID)
	require.Empty(t, page.NextPageToken)
	require.Equal(t, http.StatusBadRequest, runMemoTrashRequest(t, svc, owner, http.MethodPost, "/internal/trash/"+expired.UID+"/restore").Code)
	subscriber := svc.SSEHub.Subscribe()
	defer svc.SSEHub.Unsubscribe(subscriber)
	require.NoError(t, svc.PurgeExpiredMemoTrash(ctx, now))
	requireMemoChanged(t, subscriber.events)
	lookup := func(memo *store.Memo) *store.Memo {
		t.Helper()
		stored, err := svc.Store.GetMemo(ctx, &store.FindMemo{ID: &memo.ID, Trash: store.MemoTrashInclude})
		require.NoError(t, err)
		return stored
	}
	require.Nil(t, lookup(expired))
	require.NotNil(t, lookup(older))
	require.NotNil(t, lookup(newer))
}
