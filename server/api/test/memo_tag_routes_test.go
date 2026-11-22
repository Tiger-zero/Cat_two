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

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/store"
)

func TestMemoTagEditsThroughREST(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	t.Cleanup(ts.Cleanup)
	handler, err := ts.Service.NewHandler(ctx)
	require.NoError(t, err)

	owner, err := ts.CreateHostUser(ctx, "tag-editor")
	require.NoError(t, err)
	otherOwner, err := ts.CreateRegularUser(ctx, "other-note-owner")
	require.NoError(t, err)
	personalAccessToken := func(user *store.User) string {
		t.Helper()
		created, err := ts.Service.CreatePersonalAccessToken(ts.CreateUserContext(ctx, user.ID), &apipb.CreatePersonalAccessTokenRequest{
			Parent:      "users/" + user.Username,
			Description: "tag editor test client",
		})
		require.NoError(t, err)
		require.NotEmpty(t, created.GetToken())
		return created.GetToken()
	}
	ownerToken := personalAccessToken(owner)
	otherOwnerToken := personalAccessToken(otherOwner)

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
	createMemo := func(token, content string) *apipb.Memo {
		t.Helper()
		const createdAt = "2024-08-01T10:00:00Z"
		const updatedAt = "2024-08-02T12:30:00Z"
		memo := requireMemoResponse(request(http.MethodPost, "/api/memos", token, map[string]string{
			"content":    content,
			"visibility": "PRIVATE",
			"createTime": createdAt,
			"updateTime": updatedAt,
		}))
		require.Equal(t, createdAt, memo.GetCreateTime().AsTime().Format(time.RFC3339))
		require.Equal(t, updatedAt, memo.GetUpdateTime().AsTime().Format(time.RFC3339))
		return memo
	}
	requireMemo := func(token string, original *apipb.Memo, content string) *apipb.Memo {
		t.Helper()
		memo := requireMemoResponse(request(http.MethodGet, "/api/"+original.GetName(), token, nil))
		require.Equal(t, original.GetName(), memo.GetName())
		require.Equal(t, original.GetCreator(), memo.GetCreator())
		require.Equal(t, original.GetVisibility(), memo.GetVisibility())
		require.Equal(t, original.GetCreateTime(), memo.GetCreateTime())
		require.Equal(t, original.GetUpdateTime(), memo.GetUpdateTime())
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

	source := createMemo(ownerToken, "Keep this #2027/08 note.\n\n`#2027/08` is code.")
	descendant := createMemo(ownerToken, "Child #2027/08/project")
	destination := createMemo(ownerToken, "Already at #2027/09")
	unrelated := createMemo(ownerToken, "Different tag #2027/080")
	needsNormalization := createMemo(ownerToken, "Review #2027/ 08 and `#2027/ 08`")
	otherMemo := createMemo(otherOwnerToken, "Other account #2027/08 and #2027/ 08")

	rename := map[string]string{"oldName": "#2027/08", "newName": "#2027/09"}
	require.Equal(t, http.StatusUnauthorized, request(http.MethodPost, "/api/tags/rename", "", rename).Code)
	require.Equal(t, http.StatusUnauthorized, request(http.MethodPost, "/api/tags/normalize", "", nil).Code)

	requireCount(request(http.MethodPost, "/api/tags/rename", ownerToken, rename), 2)
	merged := requireMemo(ownerToken, source, "Keep this #2027/09 note.\n\n`#2027/08` is code.")
	require.ElementsMatch(t, []string{"2027", "2027/09"}, merged.GetTags())
	mergedChild := requireMemo(ownerToken, descendant, "Child #2027/09/project")
	require.ElementsMatch(t, []string{"2027", "2027/09", "2027/09/project"}, mergedChild.GetTags())
	requireMemo(ownerToken, destination, destination.GetContent())
	requireMemo(ownerToken, unrelated, unrelated.GetContent())
	requireMemo(ownerToken, needsNormalization, needsNormalization.GetContent())
	requireMemo(otherOwnerToken, otherMemo, otherMemo.GetContent())
	requireCount(request(http.MethodPost, "/api/tags/rename", ownerToken, rename), 0)

	requireCount(request(http.MethodPost, "/api/tags/normalize", ownerToken, nil), 1)
	normalized := requireMemo(ownerToken, needsNormalization, "Review #2027/08 and `#2027/ 08`")
	require.ElementsMatch(t, []string{"2027", "2027/08"}, normalized.GetTags())
	requireMemo(otherOwnerToken, otherMemo, otherMemo.GetContent())
	requireCount(request(http.MethodPost, "/api/tags/normalize", ownerToken, nil), 0)

	remove := map[string]string{"tagName": "#2027/09", "mode": "remove"}
	require.Equal(t, http.StatusUnauthorized, request(http.MethodPost, "/api/tags/delete", "", remove).Code)
	require.Equal(t, http.StatusBadRequest, request(http.MethodPost, "/api/tags/delete", ownerToken, map[string]string{"tagName": "2027/09", "mode": "purge"}).Code)
	require.Equal(t, http.StatusBadRequest, request(http.MethodPost, "/api/tags/delete", ownerToken, map[string]string{"tagName": "not a tag", "mode": "remove"}).Code)
	archivedChild := requireMemoResponse(request(http.MethodPatch, "/api/"+mergedChild.GetName()+"?updateMask=state", ownerToken, map[string]string{"state": "ARCHIVED"}))
	requireCount(request(http.MethodPost, "/api/tags/delete", ownerToken, remove), 3)
	removed := requireMemo(ownerToken, merged, "Keep this note.\n\n`#2027/08` is code.")
	require.Empty(t, removed.GetTags())
	require.NotEqual(t, merged.GetRevision(), removed.GetRevision())
	removedChild := requireMemo(ownerToken, archivedChild, "Child ")
	require.Equal(t, apipb.State_ARCHIVED, removedChild.GetState())
	require.Empty(t, removedChild.GetTags())
	requireMemo(ownerToken, unrelated, unrelated.GetContent())
	requireMemo(otherOwnerToken, otherMemo, otherMemo.GetContent())
	requireCount(request(http.MethodPost, "/api/tags/delete", ownerToken, remove), 0)

	attachment := new(apipb.Attachment)
	createdFile := request(http.MethodPost, "/api/attachments", ownerToken, map[string]string{
		"filename": "tag-test.txt", "type": "text/plain", "content": "ZmlsZQ==",
	})
	require.Equal(t, http.StatusOK, createdFile.Code, "%s", createdFile.Body)
	require.NoError(t, protojson.Unmarshal(createdFile.Body.Bytes(), attachment))
	root := requireMemoResponse(request(http.MethodPost, "/api/memos", ownerToken, map[string]any{
		"content":     "Parent #recycle/project",
		"attachments": []map[string]string{{"name": attachment.GetName()}},
	}))
	reply := requireMemoResponse(request(http.MethodPost, "/api/"+root.GetName()+"/comments", ownerToken, map[string]string{"content": "Reply"}))
	nested := requireMemoResponse(request(http.MethodPost, "/api/"+reply.GetName()+"/comments", ownerToken, map[string]string{"content": "Nested reply"}))
	root = requireMemoResponse(request(http.MethodPatch, "/api/"+root.GetName()+"?updateMask=state", ownerToken, map[string]string{"state": "ARCHIVED"}))
	stillOutside := createMemo(ownerToken, "Different #recycle-bin")
	otherOwnerMatching := createMemo(otherOwnerToken, "Different owner #recycle/project")
	trash := map[string]string{"tagName": "recycle", "mode": "trash"}
	requireCount(request(http.MethodPost, "/api/tags/delete", ownerToken, trash), 1)
	for _, memo := range []*apipb.Memo{root, reply, nested} {
		require.Equal(t, http.StatusNotFound, request(http.MethodGet, "/api/"+memo.GetName(), ownerToken, nil).Code)
	}
	requireMemo(ownerToken, stillOutside, stillOutside.GetContent())
	requireMemo(otherOwnerToken, otherOwnerMatching, otherOwnerMatching.GetContent())
	requireCount(request(http.MethodPost, "/api/tags/delete", ownerToken, trash), 0)
	restorePath := "/api/trash/" + strings.TrimPrefix(root.GetName(), "memos/") + "/restore"
	require.Equal(t, http.StatusNoContent, request(http.MethodPost, restorePath, ownerToken, nil).Code)
	restored := requireMemo(ownerToken, root, root.GetContent())
	require.Equal(t, apipb.State_ARCHIVED, restored.GetState())
	require.ElementsMatch(t, []string{"recycle", "recycle/project"}, restored.GetTags())
	require.Len(t, restored.GetAttachments(), 1)
	require.Equal(t, attachment.GetName(), restored.GetAttachments()[0].GetName())
	requireMemo(ownerToken, reply, reply.GetContent())
	requireMemo(ownerToken, nested, nested.GetContent())
	removeRestored := map[string]string{"tagName": "recycle", "mode": "remove"}
	requireCount(request(http.MethodPost, "/api/tags/delete", ownerToken, removeRestored), 1)
	untagged := requireMemo(ownerToken, restored, "Parent ")
	require.Equal(t, apipb.State_ARCHIVED, untagged.GetState())
	require.Empty(t, untagged.GetTags())
	require.Len(t, untagged.GetAttachments(), 1)
	require.Equal(t, attachment.GetName(), untagged.GetAttachments()[0].GetName())
}
