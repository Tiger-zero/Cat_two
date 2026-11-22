package test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestPersonalMCPAndRESTShareMemoWriteContract(t *testing.T) {
	ctx := t.Context()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	webToken := inst.signIn(t)
	_, token := createMCPConnection(t, inst, webToken, "Typed write contract", "")
	session, _ := connectMCP(ctx, t, inst, token)

	const uid = "mcp-shared-write"
	payload := map[string]any{"content": "# Original\n\n#shared", "visibility": "PRIVATE"}
	created, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memo_create_memo",
		Arguments: map[string]any{
			"memoId": uid, "idempotent": true, "body": payload,
		},
	})
	require.NoError(t, err)
	require.False(t, created.IsError, "%v", created.Content)
	createdJSON, err := json.Marshal(created.StructuredContent)
	require.NoError(t, err)
	var createdMemo struct{ Name, Revision string }
	require.NoError(t, json.Unmarshal(createdJSON, &createdMemo))
	require.Equal(t, "memos/"+uid, createdMemo.Name)
	require.NotEmpty(t, createdMemo.Revision)

	status, response := inst.do(t, http.MethodPost, "/api/memos?memoId="+uid+"&idempotent=true", webToken, payload)
	require.Equal(t, http.StatusOK, status, "%s", response)
	require.JSONEq(t, string(createdJSON), string(response))
	status, response = inst.do(t, http.MethodPatch, "/api/memos/"+uid+"?updateMask=content&expectedRevision="+url.QueryEscape(createdMemo.Revision), webToken,
		map[string]string{"content": "Updated through REST"})
	require.Equal(t, http.StatusOK, status, "%s", response)

	stale, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memo_update_memo",
		Arguments: map[string]any{
			"memo": createdMemo.Name, "expectedRevision": createdMemo.Revision,
			"body": map[string]string{"content": "Stale edit"},
		},
	})
	require.NoError(t, err)
	require.True(t, stale.IsError, "%v", stale.Content)
	require.Contains(t, stale.Content[0].(*sdkmcp.TextContent).Text, "409 Conflict")

	read, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: "memo_get_memo", Arguments: map[string]string{"memo": uid}})
	require.NoError(t, err)
	require.False(t, read.IsError, "%v", read.Content)
	readJSON, err := json.Marshal(read.StructuredContent)
	require.NoError(t, err)
	require.JSONEq(t, string(response), string(readJSON))
}
