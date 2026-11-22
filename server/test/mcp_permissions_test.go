package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestPersonalMCPReadOnlyPermission(t *testing.T) {
	ctx := t.Context()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	browserToken := inst.signIn(t)
	inst.createMemo(t, browserToken, "read-only-fixture", "A private note")

	status, body := inst.do(t, http.MethodPost, mcpConnectionsPath, browserToken, map[string]string{"label": "Unknown", "permission": "write_only"})
	require.Equal(t, http.StatusBadRequest, status, "%s", body)
	status, body = inst.do(t, http.MethodPost, mcpConnectionsPath, browserToken, map[string]string{"label": "Reader", "permission": "read_only"})
	require.Equal(t, http.StatusCreated, status, "%s", body)
	var issued struct {
		Connection observedMCPConnection `json:"connection"`
		Token      string                `json:"token"`
	}
	require.NoError(t, json.Unmarshal(body, &issued))
	require.Equal(t, "read_only", issued.Connection.Permission)
	require.Equal(t, "read_only", getMCPConnection(t, inst, browserToken, issued.Connection.ID, issued.Token).Permission)

	reader, _ := connectMCP(ctx, t, inst, issued.Token)
	list, err := reader.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Equal(t, "private", list.CacheScope)
	var names []string
	for _, tool := range list.Tools {
		names = append(names, tool.Name)
	}
	require.ElementsMatch(t, []string{
		"auth_get_current_user", "memo_list_memos", "memo_get_memo", "memo_search", "memo_batch_get", "tag_tree", "tag_search",
	}, names)

	discover := `{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"}}}}`
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, inst.baseURL+"/mcp", bytes.NewBufferString(discover))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Authorization", "Bearer "+issued.Token)
	request.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	request.Header.Set("Mcp-Method", "server/discover")
	response, err := inst.client.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var discovery struct{ Result sdkmcp.DiscoverResult }
	require.NoError(t, json.NewDecoder(response.Body).Decode(&discovery))
	require.Equal(t, "private", discovery.Result.CacheScope)
	require.Contains(t, discovery.Result.Instructions, "read-only")

	result, err := reader.CallTool(ctx, &sdkmcp.CallToolParams{Name: "memo_get_memo", Arguments: map[string]any{"memo": "read-only-fixture"}})
	require.NoError(t, err)
	require.False(t, result.IsError, "%v", result.Content)
	for _, name := range []string{"memo_create_memo", "memo_update_memo"} {
		arguments := map[string]any{"body": map[string]any{"content": "unexpected"}}
		if name == "memo_update_memo" {
			arguments["memo"] = "read-only-fixture"
		}
		result, err := reader.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: arguments})
		require.NoError(t, err)
		require.True(t, result.IsError)
		require.Equal(t, "this MCP connection has read-only access", result.Content[0].(*sdkmcp.TextContent).Text)
	}
	inst.requireMemo(t, browserToken, "read-only-fixture", "A private note")
	_, writerToken := createMCPConnection(t, inst, browserToken, "Writer", "")
	writer, _ := connectMCP(ctx, t, inst, writerToken)
	writerTools, err := writer.ListTools(ctx, nil)
	require.NoError(t, err)
	require.Len(t, writerTools.Tools, len(list.Tools)+2)
}

func TestPersonalMCPBatchMarkdownBudget(t *testing.T) {
	ctx := t.Context()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	owner, err := inst.server.Store.GetUser(ctx, &store.FindUser{Username: new(testAdminUsername)})
	require.NoError(t, err)
	// Each note fits, but 2 * 699051 three-byte runes exceed the aggregate
	// Markdown budget by two bytes. Counting runes would allow the batch.
	content := strings.Repeat("你", 699051)
	for _, uid := range []string{"large-note-a", "large-note-b"} {
		_, err := inst.server.Store.CreateMemo(ctx, &store.Memo{UID: uid, CreatorID: owner.ID, Content: content, Visibility: store.Private})
		require.NoError(t, err)
	}
	_, token := createMCPConnection(t, inst, inst.signIn(t), "Batch client", "")
	client, _ := connectMCP(ctx, t, inst, token)
	result, err := client.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memo_batch_get", Arguments: map[string]any{"names": []string{"memos/large-note-a", "memos/large-note-b"}},
	})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Contains(t, result.Content[0].(*sdkmcp.TextContent).Text, "request fewer notes")
	require.Nil(t, result.StructuredContent)
}
