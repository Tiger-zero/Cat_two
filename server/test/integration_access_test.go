package test

import (
	"encoding/json"
	"net/http"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/usememos/memos/store"
)

func TestIntegrationClientsUseSameNoteOwnershipThroughRESTAndMCP(t *testing.T) {
	ctx := t.Context()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	adminSession := inst.signIn(t)

	const otherUsername = "note-owner"
	const otherMemoID = "other-private-note"
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(testAdminPassword), bcrypt.MinCost)
	require.NoError(t, err)
	_, err = inst.server.Store.CreateUser(ctx, &store.User{Username: otherUsername, Role: store.RoleUser, PasswordHash: string(passwordHash)})
	require.NoError(t, err)
	otherSession := inst.signInUser(t, otherUsername, testAdminPassword)
	inst.createMemo(t, otherSession, otherMemoID, "Original private note")

	status, body := inst.do(t, http.MethodPost, "/api/users/"+testAdminUsername+"/personalAccessTokens", adminSession, map[string]any{
		"description":   "client integration",
		"expiresInDays": 1,
	})
	require.Equal(t, http.StatusOK, status, "%s", body)
	var createdToken struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(body, &createdToken))
	require.NotEmpty(t, createdToken.Token)
	pat := createdToken.Token
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
	inst.requireMemo(t, pat, memoID, "Created with MCP\n\n#MCP连接")

	updated := call("memo_update_memo", map[string]any{
		"memo": memoID,
		"body": map[string]any{"content": "Updated with MCP"},
	})
	require.False(t, updated.IsError, "%v", updated.Content)
	inst.requireMemo(t, pat, memoID, "Updated with MCP")

	owned := call("memo_get_memo", map[string]any{"memo": memoID})
	require.False(t, owned.IsError, "%v", owned.Content)
	note, ok := owned.StructuredContent.(map[string]any)
	require.True(t, ok, "expected a note object, got %T", owned.StructuredContent)
	require.Equal(t, "Updated with MCP", note["content"])

	status, _ = inst.do(t, http.MethodGet, "/api/memos/"+otherMemoID, pat, nil)
	require.Equal(t, http.StatusForbidden, status)
	require.True(t, call("memo_get_memo", map[string]any{"memo": otherMemoID}).IsError)
	require.True(t, call("memo_update_memo", map[string]any{
		"memo": otherMemoID,
		"body": map[string]any{"content": "Unauthorised update"},
	}).IsError)
	inst.requireMemo(t, otherSession, otherMemoID, "Original private note")
}

type integrationTestTransport func(*http.Request) (*http.Response, error)

func (f integrationTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
