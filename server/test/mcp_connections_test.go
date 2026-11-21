package test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync/atomic"
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

func createMCPConnection(t *testing.T, inst *instance, browserToken, label, duration string) (observedMCPConnection, string) {
	t.Helper()
	request := map[string]string{"label": label}
	if duration != "" {
		request["duration"] = duration
	}
	status, body := inst.do(t, http.MethodPost, mcpConnectionsPath, browserToken, request)
	require.Equal(t, http.StatusCreated, status, "%s", body)
	var created struct {
		Connection observedMCPConnection `json:"connection"`
		Token      string                `json:"token"`
	}
	require.NoError(t, json.Unmarshal(body, &created))
	require.NotEmpty(t, created.Token)
	require.NotEmpty(t, created.Connection.ID)
	require.Equal(t, label, created.Connection.Label)
	require.Equal(t, "approved", created.Connection.Status)
	require.Positive(t, created.Connection.CreatedAt)
	require.Zero(t, created.Connection.RequestedAt)
	require.Empty(t, created.Connection.RemoteIP)
	createdAt := time.Unix(created.Connection.CreatedAt, 0).UTC()
	var expectedExpiresAt int64
	switch duration {
	case "", "7d":
		expectedExpiresAt = createdAt.Add(7 * 24 * time.Hour).Unix()
	case "14d":
		expectedExpiresAt = createdAt.Add(14 * 24 * time.Hour).Unix()
	case "1y":
		expectedExpiresAt = createdAt.AddDate(1, 0, 0).Unix()
	case "permanent":
		// Permanent connections have no expiration.
	default:
		t.Fatalf("invalid duration in test fixture %q", duration)
	}
	require.Equal(t, expectedExpiresAt, created.Connection.ExpiresAt)
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

func sendMCPInitialize(t *testing.T, inst *instance, token string) (int, []byte) {
	t.Helper()
	return inst.do(t, http.MethodPost, "/mcp", token, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]string{"name": "connection-test", "version": "1"},
		},
	})
}

func connectMCP(ctx context.Context, t *testing.T, inst *instance, token string) (*sdkmcp.ClientSession, *atomic.Int32) {
	t.Helper()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "connection-integration-test", Version: "1"}, nil)
	var lastHTTPStatus atomic.Int32
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{
		Endpoint:             inst.baseURL + "/mcp",
		DisableStandaloneSSE: true,
		HTTPClient: &http.Client{
			Timeout: inst.client.Timeout,
			Transport: integrationTestTransport(func(request *http.Request) (*http.Response, error) {
				req := request.Clone(request.Context())
				req.Header.Set("Authorization", "Bearer "+token)
				response, err := http.DefaultTransport.RoundTrip(req)
				if err == nil && req.Method == http.MethodPost {
					lastHTTPStatus.Store(int32(response.StatusCode))
				}
				return response, err
			}),
		},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	return session, &lastHTTPStatus
}

func TestPersonalMCPConnectionImmediateAccess(t *testing.T) {
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
	status, body = inst.do(t, http.MethodPost, mcpConnectionsPath, browserToken, map[string]string{
		"label": "Invalid duration", "duration": "week",
	})
	require.Equal(t, http.StatusBadRequest, status, "%s", body)

	// Omitting duration gives a seven-day grant from creation.
	connection, token := createMCPConnection(t, inst, browserToken, "Desktop assistant", "")
	connectionPath := mcpConnectionsPath + "/" + connection.ID
	status, _ = inst.do(t, http.MethodDelete, connectionPath, "", nil)
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = inst.do(t, http.MethodDelete, connectionPath, pat.Token, nil)
	require.Equal(t, http.StatusForbidden, status)
	status, _ = inst.do(t, http.MethodDelete, connectionPath, token, nil)
	require.Equal(t, http.StatusUnauthorized, status)
	for _, action := range []string{"approve_day", "approve_permanent", "deny"} {
		status, body = mcpDecision(t, inst, browserToken, connection.ID, action)
		require.Equal(t, http.StatusBadRequest, status, "%s", body)
	}

	for _, ineligible := range []string{"", pat.Token, browserToken} {
		status, _ = sendMCPInitialize(t, inst, ineligible)
		require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	}
	inst.do(t, http.MethodGet, "/mcp", token, nil)
	require.Equal(t, connection, getMCPConnection(t, inst, browserToken, connection.ID, token))
	status, _ = mcpDecision(t, inst, pat.Token, connection.ID, "revoke")
	require.Equal(t, http.StatusForbidden, status)

	// Existing accounts can still sign in after an upgrade. Their sessions
	// must neither enumerate, revoke, nor delete another account's connections.
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
	status, _ = mcpDecision(t, inst, legacyToken, connection.ID, "revoke")
	require.Contains(t, []int{http.StatusConflict, http.StatusNotFound}, status)
	status, _ = inst.do(t, http.MethodDelete, connectionPath, legacyToken, nil)
	require.Equal(t, http.StatusNotFound, status)

	mcp, lastHTTPStatus := connectMCP(ctx, t, inst, token)
	firstUse := getMCPConnection(t, inst, browserToken, connection.ID, token)
	require.Equal(t, "approved", firstUse.Status)
	require.Equal(t, connection.ExpiresAt, firstUse.ExpiresAt)
	require.GreaterOrEqual(t, firstUse.RequestedAt, connection.CreatedAt)
	require.Equal(t, "127.0.0.1", firstUse.RemoteIP)
	require.Eventually(t, func() bool { return time.Now().Unix() > firstUse.RequestedAt }, 2*time.Second, 25*time.Millisecond)
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
	const memoID = "integration-note"
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
	require.Equal(t, firstUse, getMCPConnection(t, inst, browserToken, connection.ID, token))

	status, _ = inst.do(t, http.MethodGet, "/api/memos/"+memoID, token, nil)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	status, _ = inst.do(t, http.MethodGet, mcpConnectionsPath, token, nil)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	status, _ = mcpDecision(t, inst, token, connection.ID, "revoke")
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	inst.requireMemo(t, pat.Token, memoID, "Updated by the integration")

	status, body = inst.do(t, http.MethodDelete, connectionPath, browserToken, nil)
	require.Equal(t, http.StatusNoContent, status, "%s", body)
	status, body = inst.do(t, http.MethodGet, mcpConnectionsPath, browserToken, nil)
	require.Equal(t, http.StatusOK, status, "%s", body)
	require.JSONEq(t, `{"connections":[]}`, string(body))
	status, _ = inst.do(t, http.MethodDelete, connectionPath, browserToken, nil)
	require.Equal(t, http.StatusNotFound, status)
	// An initialized SDK client can cache tool descriptions. A tool call
	// always crosses the transport and must observe the deleted token.
	_, err = mcp.CallTool(ctx, &sdkmcp.CallToolParams{Name: "memo_get_memo", Arguments: map[string]any{"memo": memoID}})
	require.Error(t, err, "deletion must also cut off an initialized client")
	require.EqualValues(t, http.StatusUnauthorized, lastHTTPStatus.Load())
	status, _ = sendMCPInitialize(t, inst, token)
	require.Equal(t, http.StatusUnauthorized, status)

	_, legacyConnectionToken := createMCPConnection(t, inst, legacyToken, "Legacy account client", "permanent")
	legacyMCP, legacyHTTPStatus := connectMCP(ctx, t, inst, legacyConnectionToken)
	legacyRead := &sdkmcp.CallToolParams{Name: "memo_get_memo", Arguments: map[string]any{"memo": legacyMemoID}}
	result, err := legacyMCP.CallTool(ctx, legacyRead)
	require.NoError(t, err)
	require.False(t, result.IsError, "%v", result.Content)
	require.EqualValues(t, http.StatusOK, legacyHTTPStatus.Load())

	// Archive through the driver so a warm Store cache can still hold the
	// account as active. The next MCP request must read its database state.
	archived := store.Archived
	archivedOwner, err := inst.server.Store.GetDriver().UpdateUser(ctx, &store.UpdateUser{ID: legacy.ID, RowStatus: &archived})
	require.NoError(t, err)
	require.Equal(t, store.Archived, archivedOwner.RowStatus)
	_, err = legacyMCP.CallTool(ctx, legacyRead)
	require.Error(t, err, "archiving the owner must cut off an initialized client")
	require.EqualValues(t, http.StatusUnauthorized, legacyHTTPStatus.Load())
}

func TestPersonalMCPConnectionsSurviveRestartAndKeepLegacyAccess(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	inst := bootInstance(ctx, t, instanceOptions{personal: true, dataDir: dataDir})
	inst.createAdmin(t)
	browserToken := inst.signIn(t)
	permanentConnection, permanentToken := createMCPConnection(t, inst, browserToken, "Always allowed", "permanent")
	expiredConnection, expiredToken := createMCPConnection(t, inst, browserToken, "Expired client", "7d")
	createMCPConnection(t, inst, browserToken, "Two weeks", "14d")
	createMCPConnection(t, inst, browserToken, "One year", "1y")

	username := testAdminUsername
	owner, err := inst.server.Store.GetUser(ctx, &store.FindUser{Username: &username})
	require.NoError(t, err)
	require.NotNil(t, owner)
	// Exercise expiration without sleeping for a day or depending on SQLite's table layout.
	updated, err := inst.server.Store.UpdateMCPConnection(
		ctx, owner.ID, expiredConnection.ID, store.MCPConnectionApproved, store.MCPConnectionApproved, time.Now().Add(-time.Minute).Unix(),
	)
	require.NoError(t, err)
	require.True(t, updated)

	// Use the same token and persisted fields as the pre-upgrade issuance flow.
	legacyCreated := time.Now().Add(-time.Hour).Unix()
	legacyExpires := legacyCreated + int64(24*time.Hour/time.Second)
	legacy := map[store.MCPConnectionStatus]string{}
	for _, state := range []store.MCPConnectionStatus{
		store.MCPConnectionApproved, store.MCPConnectionAwaiting, store.MCPConnectionPending, store.MCPConnectionDenied,
	} {
		token := "cat_two_mcp_" + rand.Text()
		digest := sha256.Sum256([]byte(token))
		connection := &store.MCPConnection{
			ID: "legacy-" + string(state), UserID: owner.ID, Label: "Legacy " + string(state),
			TokenHash: hex.EncodeToString(digest[:]), Status: state, CreatedAt: legacyCreated,
		}
		if state != store.MCPConnectionAwaiting {
			connection.RequestedAt = legacyCreated + 60
			connection.RemoteIP = "192.0.2.55"
		}
		if state == store.MCPConnectionApproved {
			connection.ExpiresAt = legacyExpires
		}
		require.NoError(t, inst.server.Store.CreateMCPConnection(ctx, connection))
		legacy[state] = token
	}

	inst.shutdown(ctx)
	inst = bootInstance(ctx, t, instanceOptions{personal: true, dataDir: dataDir})
	browserToken = inst.signIn(t)
	permanent := getMCPConnection(t, inst, browserToken, permanentConnection.ID, permanentToken)
	require.Equal(t, "approved", permanent.Status)
	require.Zero(t, permanent.ExpiresAt)
	permanentSession, _ := connectMCP(ctx, t, inst, permanentToken)
	_, err = permanentSession.ListTools(ctx, nil)
	require.NoError(t, err)
	status, body := mcpDecision(t, inst, browserToken, permanentConnection.ID, "revoke")
	require.Equal(t, http.StatusNoContent, status, "%s", body)
	require.Equal(t, "revoked", getMCPConnection(t, inst, browserToken, permanentConnection.ID, permanentToken).Status)
	status, _ = sendMCPInitialize(t, inst, permanentToken)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)

	legacyApproved, _ := connectMCP(ctx, t, inst, legacy[store.MCPConnectionApproved])
	_, err = legacyApproved.ListTools(ctx, nil)
	require.NoError(t, err)
	approved := getMCPConnection(t, inst, browserToken, "legacy-approved", legacy[store.MCPConnectionApproved])
	require.Equal(t, "approved", approved.Status)
	require.Equal(t, legacyCreated+60, approved.RequestedAt)
	require.Equal(t, legacyExpires, approved.ExpiresAt)
	require.Equal(t, "192.0.2.55", approved.RemoteIP)

	for _, state := range []store.MCPConnectionStatus{store.MCPConnectionAwaiting, store.MCPConnectionPending, store.MCPConnectionDenied} {
		token := legacy[state]
		id := "legacy-" + string(state)
		status, _ := sendMCPInitialize(t, inst, token)
		require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
		require.Equal(t, string(state), getMCPConnection(t, inst, browserToken, id, token).Status)
		if state != store.MCPConnectionDenied {
			status, body := mcpDecision(t, inst, browserToken, id, "revoke")
			require.Equal(t, http.StatusNoContent, status, "%s", body)
			require.Equal(t, "revoked", getMCPConnection(t, inst, browserToken, id, token).Status)
			status, _ = sendMCPInitialize(t, inst, token)
			require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
		}
	}
	status, _ = sendMCPInitialize(t, inst, expiredToken)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	require.Equal(t, "expired", getMCPConnection(t, inst, browserToken, expiredConnection.ID, expiredToken).Status)
	status, body = inst.do(t, http.MethodDelete, mcpConnectionsPath+"/"+expiredConnection.ID, browserToken, nil)
	require.Equal(t, http.StatusNoContent, status, "%s", body)
	status, _ = sendMCPInitialize(t, inst, expiredToken)
	require.Equal(t, http.StatusUnauthorized, status)
}
