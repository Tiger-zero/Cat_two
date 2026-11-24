package test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/proto/gen/api/apiconnect"
)

func TestPersonalNotesAndFilesRequireBrowserSession(t *testing.T) {
	instance := bootInstance(t.Context(), t, instanceOptions{personal: true})
	instance.createAdmin(t)
	token := instance.signIn(t)
	const noteID = "private-note"
	const noteContent = "private note content"
	instance.createMemo(t, token, noteID, noteContent)

	const filename = "private.txt"
	const fileContent = "private attachment"
	status, body := instance.connect(t, apiconnect.AttachmentServiceCreateAttachmentProcedure, token, map[string]any{
		"attachment": map[string]any{
			"filename": filename, "type": "text/plain", "memo": "memos/" + noteID, "content": []byte(fileContent),
		},
	})
	require.Equal(t, http.StatusOK, status, "%s", body)
	var created struct{ Name string }
	require.NoError(t, json.Unmarshal(body, &created))
	require.True(t, strings.HasPrefix(created.Name, "attachments/"), "%s", body)

	for _, request := range []struct {
		procedure string
		name      string
		want      string
	}{
		{apiconnect.MemoServiceGetMemoProcedure, "memos/" + noteID, noteContent},
		{apiconnect.AttachmentServiceGetAttachmentProcedure, created.Name, filename},
	} {
		input := map[string]string{"name": request.name}
		status, body := instance.connect(t, request.procedure, token, input)
		require.Equal(t, http.StatusOK, status, "%s", body)
		require.Contains(t, string(body), request.want)
		status, body = instance.connect(t, request.procedure, "", input)
		require.Equal(t, http.StatusUnauthorized, status, "%s", body)
		require.NotContains(t, string(body), request.want)
	}

	path := "/file/" + created.Name
	status, body = instance.do(t, http.MethodGet, path, token, nil)
	require.Equal(t, http.StatusOK, status, "%s", body)
	require.Equal(t, fileContent, string(body))
	status, body = instance.do(t, http.MethodGet, path, "", nil)
	require.Equal(t, http.StatusUnauthorized, status, "%s", body)
	require.NotContains(t, string(body), fileContent)
}
