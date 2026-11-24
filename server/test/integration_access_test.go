package test

import (
	"net/http"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/proto/gen/api/apiconnect"
)

func TestIntegrationClientsUseSameNoteOwnershipThroughWebAndMCP(t *testing.T) {
	ctx := t.Context()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	adminSession := inst.signIn(t)

	_, connectionToken := createMCPConnection(t, inst, adminSession, "Client integration", "")
	session, _ := connectMCP(ctx, t, inst, connectionToken)

	call := func(name string, arguments map[string]any) *sdkmcp.CallToolResult {
		t.Helper()
		result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: arguments})
		require.NoError(t, err)
		return result
	}

	const memoID = "integration-owned-note"
	created := call("memo_create_memo", map[string]any{
		"memoId": memoID,
		"body":   map[string]any{"content": "Created with MCP"},
	})
	require.False(t, created.IsError, "%v", created.Content)
	inst.requireMemo(t, adminSession, memoID, "Created with MCP\n\n#MCP连接")

	updated := call("memo_update_memo", map[string]any{
		"memo": memoID,
		"body": map[string]any{"content": "Updated with MCP"},
	})
	require.False(t, updated.IsError, "%v", updated.Content)
	inst.requireMemo(t, adminSession, memoID, "Updated with MCP")

	owned := call("memo_get_memo", map[string]any{"memo": memoID})
	require.False(t, owned.IsError, "%v", owned.Content)
	note, ok := owned.StructuredContent.(map[string]any)
	require.True(t, ok, "expected a note object, got %T", owned.StructuredContent)
	require.Equal(t, "Updated with MCP", note["content"])

	// A stale assistant edit must not overwrite a later browser save.
	stale := note["revision"]
	status, response := inst.connect(t, apiconnect.MemoServiceUpdateMemoProcedure, adminSession, map[string]any{
		"memo":       map[string]string{"name": "memos/" + memoID, "content": "Browser edit"},
		"updateMask": "content", "expectedRevision": stale,
	})
	require.Equal(t, http.StatusOK, status, "%s", response)
	conflict := call("memo_update_memo", map[string]any{
		"memo": "memos/" + memoID, "body": map[string]any{"content": "Stale write"},
		"updateMask": "content", "expectedRevision": stale,
	})
	require.True(t, conflict.IsError)
	require.Len(t, conflict.Content, 1)
	failure, ok := conflict.Content[0].(*sdkmcp.TextContent)
	require.True(t, ok)
	require.Contains(t, failure.Text, "409 Conflict: memo revision changed")
	inst.requireMemo(t, adminSession, memoID, "Browser edit")
}

type integrationTestTransport func(*http.Request) (*http.Response, error)

func (f integrationTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
