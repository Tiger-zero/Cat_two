package test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestStartupNotesAndFilesAreCreatorOnly(t *testing.T) {
	ctx := context.Background()
	instance := bootInstance(ctx, t, instanceOptions{instanceURL: "https://notes.example.test"})
	instance.createAdmin(t)
	adminToken := instance.signIn(t)

	const username, password = "legacy-note-owner", "owner-test-password"
	code, body := instance.do(t, http.MethodPost, "/api/users", adminToken, map[string]any{
		"username": username, "password": password, "email": "owner@example.test",
	})
	require.Equal(t, http.StatusOK, code, "%s", body)
	code, body = instance.do(t, http.MethodPost, "/api/auth/signin", "", map[string]any{
		"passwordCredentials": map[string]string{"username": username, "password": password},
	})
	require.Equal(t, http.StatusOK, code, "%s", body)
	var ownerSession struct{ AccessToken string }
	require.NoError(t, json.Unmarshal(body, &ownerSession))
	require.NotEmpty(t, ownerSession.AccessToken)

	owner, err := instance.server.Store.GetUser(ctx, &store.FindUser{Username: new(username)})
	require.NoError(t, err)
	require.NotNil(t, owner)

	for _, visibility := range []store.Visibility{store.Public, store.Protected} {
		t.Run(string(visibility), func(t *testing.T) {
			uid := "legacy-" + string(visibility)
			memo, err := instance.server.Store.CreateMemo(ctx, &store.Memo{
				UID: uid, CreatorID: owner.ID, Content: "legacy private note content", Visibility: visibility,
			})
			require.NoError(t, err)
			content := []byte("legacy private attachment")
			attachment, err := instance.server.Store.CreateAttachment(ctx, &store.Attachment{
				UID: uid + "-file", Filename: "memo.txt", Type: "text/plain", MemoID: &memo.ID, CreatorID: owner.ID,
				Blob: content, Size: int64(len(content)),
			})
			require.NoError(t, err)
			share, err := instance.server.Store.CreateMemoShare(ctx, &store.MemoShare{
				UID: uid + "-share", MemoID: memo.ID, CreatorID: owner.ID,
			})
			require.NoError(t, err)

			code, body := instance.do(t, http.MethodGet, "/api/memos/"+memo.UID, ownerSession.AccessToken, nil)
			require.Equal(t, http.StatusOK, code, "%s", body)
			var returned struct{ Visibility string }
			require.NoError(t, json.Unmarshal(body, &returned))
			require.Equal(t, "PRIVATE", returned.Visibility)

			for _, request := range []struct {
				path string
				want string
			}{
				{"/api/memos/" + memo.UID, memo.Content},
				{"/api/attachments/" + attachment.UID, attachment.Filename},
				{"/file/attachments/" + attachment.UID, string(content)},
			} {
				code, body := instance.do(t, http.MethodGet, request.path, ownerSession.AccessToken, nil)
				require.Equal(t, http.StatusOK, code, "%s: %s", request.path, body)
				require.Contains(t, string(body), request.want)

				code, body = instance.do(t, http.MethodGet, request.path, "", nil)
				require.Equal(t, http.StatusUnauthorized, code, "%s: %s", request.path, body)
				require.NotContains(t, string(body), request.want)

				code, body = instance.do(t, http.MethodGet, request.path, adminToken, nil)
				require.Equal(t, http.StatusForbidden, code, "%s: %s", request.path, body)
				require.NotContains(t, string(body), request.want)
			}

			code, body = instance.do(t, http.MethodGet, "/api/shares/"+share.UID+"/memo", "", nil)
			require.Equal(t, http.StatusNotFound, code, "%s", body)
			require.NotContains(t, string(body), memo.Content)

			code, body = instance.do(t, http.MethodGet, "/api/memos", adminToken, nil)
			require.Equal(t, http.StatusOK, code, "%s", body)
			require.NotContains(t, string(body), memo.UID)

			code, body = instance.do(t, http.MethodGet, "/api/memos", ownerSession.AccessToken, nil)
			require.Equal(t, http.StatusOK, code, "%s", body)
			require.Contains(t, string(body), memo.UID)
		})
	}
}
