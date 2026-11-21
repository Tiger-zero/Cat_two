package test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/usememos/memos/store"
)

type observedMCPConnection struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Status      string `json:"status"`
	CreatedAt   int64  `json:"createdAt"`
	RequestedAt int64  `json:"requestedAt"`
	ExpiresAt   int64  `json:"expiresAt"`
	RemoteIP    string `json:"remoteIp"`
}

const mcpConnectionsPath = "/api/mcpConnections"

func createMCPConnection(t *testing.T, inst *instance, browserToken, label string) (observedMCPConnection, string) {
	t.Helper()
	status, body := inst.do(t, http.MethodPost, mcpConnectionsPath, browserToken, map[string]string{"label": label})
	require.Equal(t, http.StatusCreated, status, "%s", body)
	var created struct {
		Connection observedMCPConnection `json:"connection"`
		Token      string                `json:"token"`
	}
	require.NoError(t, json.Unmarshal(body, &created))
	require.NotEmpty(t, created.Token)
	require.NotEmpty(t, created.Connection.ID)
	require.Equal(t, label, created.Connection.Label)
	require.Equal(t, "awaiting", created.Connection.Status)
	require.Positive(t, created.Connection.CreatedAt)
	require.Zero(t, created.Connection.RequestedAt)
	require.Zero(t, created.Connection.ExpiresAt)
	return created.Connection, created.Token
}

func getMCPConnection(t *testing.T, inst *instance, browserToken, id, issuedToken string) observedMCPConnection {
	t.Helper()
	status, body := inst.do(t, http.MethodGet, mcpConnectionsPath, browserToken, nil)
	require.Equal(t, http.StatusOK, status, "%s", body)
	require.NotContains(t, string(body), issuedToken)
	require.NotContains(t, string(body), "tokenHash")
	var listed struct {
		Connections []observedMCPConnection `json:"connections"`
	}
	require.NoError(t, json.Unmarshal(body, &listed))
	for _, connection := range listed.Connections {
		if connection.ID == id {
			return connection
		}
	}
	t.Fatalf("connection %s missing", id)
	return observedMCPConnection{}
}

func mcpDecision(t *testing.T, inst *instance, browserToken, id, action string) (int, []byte) {
	t.Helper()
	return inst.do(t, http.MethodPost, mcpConnectionsPath+"/"+id+"/decision", browserToken, map[string]string{"action": action})
}

func requestMCPApproval(t *testing.T, inst *instance, token string) (int, []byte) {
	t.Helper()
	return inst.do(t, http.MethodPost, "/mcp", token, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]string{"name": "approval-test", "version": "1"},
		},
	})
}

func connectApprovedMCP(ctx context.Context, t *testing.T, inst *instance, token string) *sdkmcp.ClientSession {
	t.Helper()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "approved-integration-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{
		Endpoint:             inst.baseURL + "/mcp",
		DisableStandaloneSSE: true,
		HTTPClient: &http.Client{
			Timeout: inst.client.Timeout,
			Transport: integrationTestTransport(func(request *http.Request) (*http.Response, error) {
				req := request.Clone(request.Context())
				req.Header.Set("Authorization", "Bearer "+token)
				return http.DefaultTransport.RoundTrip(req)
			}),
		},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	return session
}

func TestPersonalMCPConnectionManualApproval(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	browserToken := inst.signIn(t)
	status, body := inst.do(t, http.MethodPost, "/api/users/"+testAdminUsername+"/personalAccessTokens", browserToken, map[string]string{
		"description": "REST client",
	})
	require.Equal(t, http.StatusOK, status, "%s", body)
	var pat struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(body, &pat))
	require.NotEmpty(t, pat.Token)

	status, _ = inst.do(t, http.MethodGet, mcpConnectionsPath, "", nil)
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = inst.do(t, http.MethodGet, mcpConnectionsPath, pat.Token, nil)
	require.Equal(t, http.StatusForbidden, status)
	status, _ = inst.do(t, http.MethodPost, mcpConnectionsPath, pat.Token, map[string]string{"label": "untrusted"})
	require.Equal(t, http.StatusForbidden, status)

	connection, token := createMCPConnection(t, inst, browserToken, "Desktop assistant")
	status, _ = mcpDecision(t, inst, browserToken, connection.ID, "approve_day")
	require.Contains(t, []int{http.StatusConflict, http.StatusNotFound}, status)

	status, _ = requestMCPApproval(t, inst, "")
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	status, _ = requestMCPApproval(t, inst, pat.Token)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	status, _ = requestMCPApproval(t, inst, browserToken)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	status, _ = inst.do(t, http.MethodGet, "/mcp", token, nil)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	require.Equal(t, connection, getMCPConnection(t, inst, browserToken, connection.ID, token))

	status, body = requestMCPApproval(t, inst, token)
	require.Equal(t, http.StatusForbidden, status, "%s", body)
	pending := getMCPConnection(t, inst, browserToken, connection.ID, token)
	require.Equal(t, "pending", pending.Status)
	require.GreaterOrEqual(t, pending.RequestedAt, connection.CreatedAt)
	require.Equal(t, "127.0.0.1", pending.RemoteIP)
	require.Zero(t, pending.ExpiresAt)
	require.Eventually(t, func() bool { return time.Now().Unix() > pending.RequestedAt }, 2*time.Second, 25*time.Millisecond)
	status, body = requestMCPApproval(t, inst, token)
	require.Equal(t, http.StatusForbidden, status, "%s", body)
	require.Equal(t, pending, getMCPConnection(t, inst, browserToken, connection.ID, token))
	status, _ = mcpDecision(t, inst, pat.Token, connection.ID, "approve_permanent")
	require.Equal(t, http.StatusForbidden, status)

	// Existing accounts can still sign in after an upgrade. Their sessions
	// must neither enumerate nor approve another account's connections.
	hash, err := bcrypt.GenerateFromPassword([]byte(testAdminPassword), bcrypt.MinCost)
	require.NoError(t, err)
	legacy, err := inst.server.Store.CreateUser(ctx, &store.User{
		Username: "legacy-user", Role: store.RoleUser, PasswordHash: string(hash),
	})
	require.NoError(t, err)
	legacyToken := inst.signInUser(t, legacy.Username, testAdminPassword)
	const legacyMemoID = "legacy-private-note"
	inst.createMemo(t, legacyToken, legacyMemoID, "Owned by the legacy account")
	status, body = inst.do(t, http.MethodGet, mcpConnectionsPath, legacyToken, nil)
	require.Equal(t, http.StatusOK, status, "%s", body)
	require.JSONEq(t, `{"connections":[]}`, string(body))
	status, _ = mcpDecision(t, inst, legacyToken, connection.ID, "approve_permanent")
	require.Contains(t, []int{http.StatusConflict, http.StatusNotFound}, status)
	require.Equal(t, pending, getMCPConnection(t, inst, browserToken, connection.ID, token))

	beforeApproval := time.Now()
	status, body = mcpDecision(t, inst, browserToken, connection.ID, "approve_day")
	require.Equal(t, http.StatusNoContent, status, "%s", body)
	afterApproval := time.Now()
	approved := getMCPConnection(t, inst, browserToken, connection.ID, token)
	require.Equal(t, "approved", approved.Status)
	require.GreaterOrEqual(t, approved.ExpiresAt, beforeApproval.Add(24*time.Hour).Unix())
	require.LessOrEqual(t, approved.ExpiresAt, afterApproval.Add(24*time.Hour).Unix())

	mcp := connectApprovedMCP(ctx, t, inst, token)
	tools, err := mcp.ListTools(ctx, nil)
	require.NoError(t, err)
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	require.ElementsMatch(t, []string{
		"auth_get_current_user", "user_list_memo_views", "memo_list_memos", "memo_get_memo", "memo_create_memo", "memo_update_memo",
	}, names)

	call := func(name string, args map[string]any) *sdkmcp.CallToolResult {
		t.Helper()
		result, err := mcp.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, err)
		require.False(t, result.IsError, "%v", result.Content)
		return result
	}
	const memoID = "manually-approved-note"
	call("memo_create_memo", map[string]any{
		"memoId": memoID,
		"body":   map[string]any{"content": "Written by an approved integration"},
	})
	note := call("memo_get_memo", map[string]any{"memo": memoID})
	content, ok := note.StructuredContent.(map[string]any)
	require.True(t, ok, "expected structured memo response")
	require.Equal(t, "Written by an approved integration", content["content"])
	call("memo_update_memo", map[string]any{
		"memo": memoID,
		"body": map[string]any{"content": "Updated by the integration"},
	})
	inaccessible, err := mcp.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "memo_get_memo", Arguments: map[string]any{"memo": legacyMemoID},
	})
	require.NoError(t, err)
	require.True(t, inaccessible.IsError, "approved clients inherit note ownership")

	status, _ = inst.do(t, http.MethodGet, "/api/memos/"+memoID, token, nil)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	status, _ = inst.do(t, http.MethodGet, mcpConnectionsPath, token, nil)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	status, _ = mcpDecision(t, inst, token, connection.ID, "revoke")
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	inst.requireMemo(t, pat.Token, memoID, "Updated by the integration")

	status, body = mcpDecision(t, inst, browserToken, connection.ID, "revoke")
	require.Equal(t, http.StatusNoContent, status, "%s", body)
	require.Equal(t, "revoked", getMCPConnection(t, inst, browserToken, connection.ID, token).Status)
	// An initialized SDK client can cache tool descriptions. A tool call
	// always crosses the transport and must observe the revoked token.
	_, err = mcp.CallTool(ctx, &sdkmcp.CallToolParams{Name: "memo_get_memo", Arguments: map[string]any{"memo": memoID}})
	require.Error(t, err, "revocation must also cut off an initialized client")
	status, _ = requestMCPApproval(t, inst, token)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
}

func TestPersonalMCPConnectionDecisionsSurviveRestart(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	inst := bootInstance(ctx, t, instanceOptions{personal: true, dataDir: dataDir})
	inst.createAdmin(t)
	browserToken := inst.signIn(t)
	permanentConnection, permanentToken := createMCPConnection(t, inst, browserToken, "Always allowed")
	deniedConnection, deniedToken := createMCPConnection(t, inst, browserToken, "Denied client")
	expiredConnection, expiredToken := createMCPConnection(t, inst, browserToken, "Expired client")
	for _, token := range []string{permanentToken, deniedToken, expiredToken} {
		status, body := requestMCPApproval(t, inst, token)
		require.Equal(t, http.StatusForbidden, status, "%s", body)
	}

	status, body := mcpDecision(t, inst, browserToken, permanentConnection.ID, "approve_permanent")
	require.Equal(t, http.StatusNoContent, status, "%s", body)
	status, body = mcpDecision(t, inst, browserToken, deniedConnection.ID, "deny")
	require.Equal(t, http.StatusNoContent, status, "%s", body)
	status, _ = mcpDecision(t, inst, browserToken, deniedConnection.ID, "approve_permanent")
	require.Contains(t, []int{http.StatusConflict, http.StatusNotFound}, status)

	// Move the temporary grant's expiration into the past without sleeping
	// for a day or depending on the SQLite table layout.
	username := testAdminUsername
	owner, err := inst.server.Store.GetUser(ctx, &store.FindUser{Username: &username})
	require.NoError(t, err)
	require.NotNil(t, owner)
	updated, err := inst.server.Store.UpdateMCPConnection(
		ctx, owner.ID, expiredConnection.ID, store.MCPConnectionPending, store.MCPConnectionApproved, time.Now().Add(-time.Minute).Unix(),
	)
	require.NoError(t, err)
	require.True(t, updated)

	inst.shutdown(ctx)
	inst = bootInstance(ctx, t, instanceOptions{personal: true, dataDir: dataDir})
	browserToken = inst.signIn(t)
	permanent := getMCPConnection(t, inst, browserToken, permanentConnection.ID, permanentToken)
	require.Equal(t, "approved", permanent.Status)
	require.Zero(t, permanent.ExpiresAt)
	permanentSession := connectApprovedMCP(ctx, t, inst, permanentToken)
	_, err = permanentSession.ListTools(ctx, nil)
	require.NoError(t, err)

	status, _ = requestMCPApproval(t, inst, deniedToken)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	require.Equal(t, "denied", getMCPConnection(t, inst, browserToken, deniedConnection.ID, deniedToken).Status)
	status, _ = requestMCPApproval(t, inst, expiredToken)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	require.Equal(t, "expired", getMCPConnection(t, inst, browserToken, expiredConnection.ID, expiredToken).Status)
}
