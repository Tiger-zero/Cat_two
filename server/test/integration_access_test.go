package test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestIntegrationTokenUsesSameNoteOwnershipThroughRESTAndMCP(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{})
	inst.createAdmin(t)
	adminSession := inst.signIn(t)

	const otherUsername = "note-owner"
	const otherMemoID = "other-private-note"
	status, body := inst.do(t, http.MethodPost, "/api/users", adminSession, map[string]string{
		"username": otherUsername,
		"password": testAdminPassword,
		"email":    "note-owner@example.test",
	})
	require.Equal(t, http.StatusOK, status, "%s", body)
	otherSession := inst.signInUser(t, otherUsername, testAdminPassword)
	inst.createMemo(t, otherSession, otherMemoID, "Original private note")

	status, body = inst.do(t, http.MethodPost, "/api/users/"+testAdminUsername+"/personalAccessTokens", adminSession, map[string]any{
		"description":   "client integration",
		"expiresInDays": 1,
	})
	require.Equal(t, http.StatusOK, status, "%s", body)
	var createdToken struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(body, &createdToken))
	require.NotEmpty(t, createdToken.Token)
	token := createdToken.Token

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "cat-two-integration-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{
		Endpoint:             inst.baseURL + "/mcp",
		DisableStandaloneSSE: true,
		HTTPClient: &http.Client{
			Timeout: inst.client.Timeout,
			Transport: integrationTestTransport(func(req *http.Request) (*http.Response, error) {
				request := req.Clone(req.Context())
				request.Header.Set("Authorization", "Bearer "+token)
				return http.DefaultTransport.RoundTrip(request)
			}),
		},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })

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
	inst.requireMemo(t, token, memoID, "Created with MCP\n\n#MCP连接")

	updated := call("memo_update_memo", map[string]any{
		"memo": memoID,
		"body": map[string]any{"content": "Updated with MCP"},
	})
	require.False(t, updated.IsError, "%v", updated.Content)
	inst.requireMemo(t, token, memoID, "Updated with MCP")

	owned := call("memo_get_memo", map[string]any{"memo": memoID})
	require.False(t, owned.IsError, "%v", owned.Content)
	note, ok := owned.StructuredContent.(map[string]any)
	require.True(t, ok, "expected a note object, got %T", owned.StructuredContent)
	require.Equal(t, "Updated with MCP", note["content"])

	status, _ = inst.do(t, http.MethodGet, "/api/memos/"+otherMemoID, token, nil)
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
