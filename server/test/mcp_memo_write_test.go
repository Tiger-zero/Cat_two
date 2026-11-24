package test

import (
	"encoding/json"
	"net/http"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/proto/gen/api/apiconnect"
)

func TestPersonalMCPAndConnectShareMemoWriteContract(t *testing.T) {
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
	var createdMemo apipb.Memo
	require.NoError(t, protojson.Unmarshal(createdJSON, &createdMemo))
	require.Equal(t, "memos/"+uid, createdMemo.Name)
	require.NotEmpty(t, createdMemo.Revision)

	status, response := inst.connect(t, apiconnect.MemoServiceCreateMemoProcedure, webToken, map[string]any{
		"memoId": uid, "idempotent": true, "memo": payload,
	})
	require.Equal(t, http.StatusOK, status, "%s", response)
	var replay apipb.Memo
	require.NoError(t, protojson.Unmarshal(response, &replay))
	require.True(t, proto.Equal(&createdMemo, &replay), "Connect replay returned a different memo: %s", response)
	status, response = inst.connect(t, apiconnect.MemoServiceUpdateMemoProcedure, webToken, map[string]any{
		"memo":       map[string]string{"name": createdMemo.Name, "content": "Updated through Connect"},
		"updateMask": "content", "expectedRevision": createdMemo.Revision,
	})
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
	var updatedMemo, readMemo apipb.Memo
	require.NoError(t, protojson.Unmarshal(response, &updatedMemo))
	require.NoError(t, protojson.Unmarshal(readJSON, &readMemo))
	require.True(t, proto.Equal(&updatedMemo, &readMemo), "MCP read returned a different memo: %s", readJSON)
}
