package test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

func TestMemoTagEditsThroughWeb(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	t.Cleanup(ts.Cleanup)
	handler := ts.Service.NewHandler()

	owner, err := ts.CreateHostUser(ctx, "tag-editor")
	require.NoError(t, err)
	otherOwner, err := ts.CreateRegularUser(ctx, "other-note-owner")
	require.NoError(t, err)
	accessToken := func(user *store.User) string {
		t.Helper()
		token, _, err := auth.GenerateAccessTokenV2(user.ID, user.Username, string(user.Role), string(user.RowStatus), user.PasswordHash, []byte(ts.Secret))
		require.NoError(t, err)
		return token
	}
	ownerToken := accessToken(owner)
	otherOwnerToken := accessToken(otherOwner)

	request := func(method, path, token string, payload any) *httptest.ResponseRecorder {
		t.Helper()
		var body []byte
		if payload != nil {
			var err error
			body, err = json.Marshal(payload)
			require.NoError(t, err)
		}
		req := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	requireMemoResponse := func(response *httptest.ResponseRecorder) *apipb.Memo {
		t.Helper()
		require.Equal(t, http.StatusOK, response.Code, "%s", response.Body)
		memo := new(apipb.Memo)
		require.NoError(t, protojson.Unmarshal(response.Body.Bytes(), memo))
		return memo
	}
	createMemo := func(user *store.User, content string, attachments ...*apipb.Attachment) *apipb.Memo {
		t.Helper()
		createdAt := time.Date(2024, time.August, 1, 10, 0, 0, 0, time.UTC)
		updatedAt := time.Date(2024, time.August, 2, 12, 30, 0, 0, time.UTC)
		memo, err := ts.Service.CreateMemo(ts.CreateUserContext(ctx, user.ID), &apipb.CreateMemoRequest{
			Memo: &apipb.Memo{
				Content:     content,
				Visibility:  apipb.Visibility_PRIVATE,
				CreateTime:  timestamppb.New(createdAt),
				UpdateTime:  timestamppb.New(updatedAt),
				Attachments: attachments,
			},
		})
		require.NoError(t, err)
		require.Equal(t, createdAt, memo.GetCreateTime().AsTime())
		require.Equal(t, updatedAt, memo.GetUpdateTime().AsTime())
		return memo
	}
	requireMemo := func(token string, original *apipb.Memo, content string) *apipb.Memo {
		t.Helper()
		memo := requireMemoResponse(request(http.MethodPost, "/memos.api.MemoService/GetMemo", token, map[string]string{"name": original.GetName()}))
		require.Equal(t, original.GetName(), memo.GetName())
		require.Equal(t, original.GetCreator(), memo.GetCreator())
		require.Equal(t, original.GetVisibility(), memo.GetVisibility())
		require.Equal(t, original.GetCreateTime().AsTime(), memo.GetCreateTime().AsTime())
		require.Equal(t, original.GetUpdateTime().AsTime(), memo.GetUpdateTime().AsTime())
		require.Equal(t, content, memo.GetContent())
		return memo
	}
	requireCount := func(response *httptest.ResponseRecorder, want int) {
		t.Helper()
		require.Equal(t, http.StatusOK, response.Code, "%s", response.Body)
		var result map[string]int
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
		require.Equal(t, map[string]int{"updatedCount": want}, result)
	}

	source := createMemo(owner, "Keep this #2027/08 note.\n\n`#2027/08` is code.")
	descendant := createMemo(owner, "Child #2027/08/project")
	destination := createMemo(owner, "Already at #2027/09")
	unrelated := createMemo(owner, "Different tag #2027/080")
	needsNormalization := createMemo(owner, "Review #2027/ 08 and `#2027/ 08`")
	otherMemo := createMemo(otherOwner, "Other account #2027/08 and #2027/ 08")

	rename := map[string]string{"oldName": "#2027/08", "newName": "#2027/09"}
	require.Equal(t, http.StatusUnauthorized, request(http.MethodPost, "/internal/tags/rename", "", rename).Code)
	require.Equal(t, http.StatusUnauthorized, request(http.MethodPost, "/internal/tags/normalize", "", nil).Code)

	requireCount(request(http.MethodPost, "/internal/tags/rename", ownerToken, rename), 2)
	merged := requireMemo(ownerToken, source, "Keep this #2027/09 note.\n\n`#2027/08` is code.")
	require.ElementsMatch(t, []string{"2027", "2027/09"}, merged.GetTags())
	mergedChild := requireMemo(ownerToken, descendant, "Child #2027/09/project")
	require.ElementsMatch(t, []string{"2027", "2027/09", "2027/09/project"}, mergedChild.GetTags())
	requireMemo(ownerToken, destination, destination.GetContent())
	requireMemo(ownerToken, unrelated, unrelated.GetContent())
	requireMemo(ownerToken, needsNormalization, needsNormalization.GetContent())
	requireMemo(otherOwnerToken, otherMemo, otherMemo.GetContent())
	requireCount(request(http.MethodPost, "/internal/tags/rename", ownerToken, rename), 0)

	requireCount(request(http.MethodPost, "/internal/tags/normalize", ownerToken, nil), 1)
	normalized := requireMemo(ownerToken, needsNormalization, "Review #2027/08 and `#2027/ 08`")
	require.ElementsMatch(t, []string{"2027", "2027/08"}, normalized.GetTags())
	requireMemo(otherOwnerToken, otherMemo, otherMemo.GetContent())
	requireCount(request(http.MethodPost, "/internal/tags/normalize", ownerToken, nil), 0)

	remove := map[string]string{"tagName": "#2027/09", "mode": "remove"}
	require.Equal(t, http.StatusUnauthorized, request(http.MethodPost, "/internal/tags/delete", "", remove).Code)
	require.Equal(t, http.StatusBadRequest, request(http.MethodPost, "/internal/tags/delete", ownerToken, map[string]string{"tagName": "2027/09", "mode": "purge"}).Code)
	require.Equal(t, http.StatusBadRequest, request(http.MethodPost, "/internal/tags/delete", ownerToken, map[string]string{"tagName": "not a tag", "mode": "remove"}).Code)
	requireCount(request(http.MethodPost, "/internal/tags/delete", ownerToken, remove), 3)
	removed := requireMemo(ownerToken, merged, "Keep this note.\n\n`#2027/08` is code.")
	require.Empty(t, removed.GetTags())
	require.NotEqual(t, merged.GetRevision(), removed.GetRevision())
	removedChild := requireMemo(ownerToken, mergedChild, "Child ")
	require.Empty(t, removedChild.GetTags())
	requireMemo(ownerToken, unrelated, unrelated.GetContent())
	requireMemo(otherOwnerToken, otherMemo, otherMemo.GetContent())
	requireCount(request(http.MethodPost, "/internal/tags/delete", ownerToken, remove), 0)

	attachment := new(apipb.Attachment)
	createdFile := request(http.MethodPost, "/memos.api.AttachmentService/CreateAttachment", ownerToken, map[string]any{
		"attachment": map[string]string{"filename": "tag-test.txt", "type": "text/plain", "content": "ZmlsZQ=="},
	})
	require.Equal(t, http.StatusOK, createdFile.Code, "%s", createdFile.Body)
	require.NoError(t, protojson.Unmarshal(createdFile.Body.Bytes(), attachment))
	root := createMemo(owner, "Parent #recycle/project", &apipb.Attachment{Name: attachment.GetName()})
	reply := requireMemoResponse(request(http.MethodPost, "/memos.api.MemoService/CreateMemoComment", ownerToken, map[string]any{
		"parent": root.GetName(), "comment": map[string]string{"content": "Reply"},
	}))
	nested := requireMemoResponse(request(http.MethodPost, "/memos.api.MemoService/CreateMemoComment", ownerToken, map[string]any{
		"parent": reply.GetName(), "comment": map[string]string{"content": "Nested reply"},
	}))
	stillOutside := createMemo(owner, "Different #recycle-bin")
	otherOwnerMatching := createMemo(otherOwner, "Different owner #recycle/project")
	trash := map[string]string{"tagName": "recycle", "mode": "trash"}
	requireCount(request(http.MethodPost, "/internal/tags/delete", ownerToken, trash), 1)
	for _, memo := range []*apipb.Memo{root, reply, nested} {
		require.Equal(t, http.StatusNotFound, request(http.MethodPost, "/memos.api.MemoService/GetMemo", ownerToken, map[string]string{"name": memo.GetName()}).Code)
	}
	requireMemo(ownerToken, stillOutside, stillOutside.GetContent())
	requireMemo(otherOwnerToken, otherOwnerMatching, otherOwnerMatching.GetContent())
	requireCount(request(http.MethodPost, "/internal/tags/delete", ownerToken, trash), 0)
	restorePath := "/internal/trash/" + strings.TrimPrefix(root.GetName(), "memos/") + "/restore"
	require.Equal(t, http.StatusNoContent, request(http.MethodPost, restorePath, ownerToken, nil).Code)
	restored := requireMemo(ownerToken, root, root.GetContent())
	require.ElementsMatch(t, []string{"recycle", "recycle/project"}, restored.GetTags())
	require.Len(t, restored.GetAttachments(), 1)
	require.Equal(t, attachment.GetName(), restored.GetAttachments()[0].GetName())
	requireMemo(ownerToken, reply, reply.GetContent())
	requireMemo(ownerToken, nested, nested.GetContent())
	removeRestored := map[string]string{"tagName": "recycle", "mode": "remove"}
	requireCount(request(http.MethodPost, "/internal/tags/delete", ownerToken, removeRestored), 1)
	untagged := requireMemo(ownerToken, restored, "Parent ")
	require.Empty(t, untagged.GetTags())
	require.Len(t, untagged.GetAttachments(), 1)
	require.Equal(t, attachment.GetName(), untagged.GetAttachments()[0].GetName())
}
