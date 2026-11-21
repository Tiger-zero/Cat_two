package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/server/fileserver"
	"github.com/usememos/memos/store"
)

func runMemoTrashRequest(t *testing.T, svc *APIService, user *store.User, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	handler, err := svc.NewHandler(t.Context())
	require.NoError(t, err)
	return runMemoTrashHTTP(t, handler, svc.Secret, user, method, target)
}

func runMemoTrashHTTP(t *testing.T, handler http.Handler, secret string, user *store.User, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), method, target, nil)
	if user != nil {
		token, _, err := auth.GenerateAccessTokenV2(user.ID, user.Username, string(user.Role), string(user.RowStatus), user.PasswordHash, []byte(secret))
		require.NoError(t, err)
		request.Header.Set("Authorization", "Bearer "+token)
	}
	result := httptest.NewRecorder()
	handler.ServeHTTP(result, request)
	return result
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
	path := "/api/trash/" + uid
	require.Equal(t, http.StatusUnauthorized, runMemoTrashRequest(t, svc, nil, http.MethodGet, "/api/trash").Code)
	require.Equal(t, http.StatusNotFound, runMemoTrashRequest(t, svc, admin, http.MethodPost, path+"/restore").Code)
	require.Equal(t, http.StatusNotFound, runMemoTrashRequest(t, svc, admin, http.MethodDelete, path).Code)
	adminList := runMemoTrashRequest(t, svc, admin, http.MethodGet, "/api/trash")
	require.Equal(t, http.StatusOK, adminList.Code)
	require.JSONEq(t, `{"memos":[],"nextPageToken":""}`, adminList.Body.String())
	request := runMemoTrashRequest(t, svc, owner, http.MethodGet, "/api/trash")
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
	first := runMemoTrashRequest(t, svc, owner, http.MethodGet, "/api/trash?pageSize=1")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var page memoTrashPage
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &page))
	require.Len(t, page.Memos, 1)
	require.Contains(t, string(page.Memos[0].Memo), newer.UID)
	require.NotEmpty(t, page.NextPageToken)
	second := runMemoTrashRequest(t, svc, owner, http.MethodGet, "/api/trash?pageToken="+url.QueryEscape(page.NextPageToken))
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &page))
	require.Len(t, page.Memos, 1)
	require.Contains(t, string(page.Memos[0].Memo), older.UID)
	require.Empty(t, page.NextPageToken)
	require.Equal(t, http.StatusBadRequest, runMemoTrashRequest(t, svc, owner, http.MethodPost, "/api/trash/"+expired.UID+"/restore").Code)
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
