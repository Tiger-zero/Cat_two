package test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/proto/gen/api/apiconnect"
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
	Permission  string `json:"permission"`
}

const mcpConnectionsPath = "/internal/mcpConnections"

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
	require.Equal(t, "read_write", created.Connection.Permission)
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
	const legacyPAT = "memos_pat_retired-client"
	status, _ := inst.do(t, http.MethodGet, mcpConnectionsPath, "", nil)
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = inst.do(t, http.MethodGet, mcpConnectionsPath, legacyPAT, nil)
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = inst.do(t, http.MethodPost, mcpConnectionsPath, legacyPAT, map[string]string{"label": "untrusted"})
	require.Equal(t, http.StatusUnauthorized, status)
	status, body := inst.do(t, http.MethodPost, mcpConnectionsPath, browserToken, map[string]string{
		"label": "Invalid duration", "duration": "week",
	})
	require.Equal(t, http.StatusBadRequest, status, "%s", body)

	// Omitting duration gives a seven-day grant from creation.
	connection, token := createMCPConnection(t, inst, browserToken, "Desktop assistant", "")
	connectionPath := mcpConnectionsPath + "/" + connection.ID
	status, _ = inst.do(t, http.MethodDelete, connectionPath, "", nil)
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = inst.do(t, http.MethodDelete, connectionPath, legacyPAT, nil)
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = inst.do(t, http.MethodDelete, connectionPath, token, nil)
	require.Equal(t, http.StatusUnauthorized, status)
	for _, action := range []string{"approve_day", "approve_permanent", "deny"} {
		status, body = mcpDecision(t, inst, browserToken, connection.ID, action)
		require.Equal(t, http.StatusBadRequest, status, "%s", body)
	}

	for _, ineligible := range []string{"", legacyPAT, browserToken} {
		status, _ = sendMCPInitialize(t, inst, ineligible)
		require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	}
	inst.do(t, http.MethodGet, "/mcp", token, nil)
	require.Equal(t, connection, getMCPConnection(t, inst, browserToken, connection.ID, token))
	status, _ = mcpDecision(t, inst, legacyPAT, connection.ID, "revoke")
	require.Equal(t, http.StatusUnauthorized, status)

	mcp, lastHTTPStatus := connectMCP(ctx, t, inst, token)
	firstUse := getMCPConnection(t, inst, browserToken, connection.ID, token)
	require.Equal(t, "approved", firstUse.Status)
	require.Equal(t, connection.ExpiresAt, firstUse.ExpiresAt)
	require.GreaterOrEqual(t, firstUse.RequestedAt, connection.CreatedAt)
	require.Equal(t, "127.0.0.1", firstUse.RemoteIP)
	require.Eventually(t, func() bool { return time.Now().Unix() > firstUse.RequestedAt }, 2*time.Second, 25*time.Millisecond)
	tools, err := mcp.ListTools(ctx, nil)
	require.NoError(t, err)
	newReadTools := []string{"memo_search", "memo_batch_get", "tag_tree", "tag_search"}
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		if slices.Contains(newReadTools, tool.Name) {
			require.NotNil(t, tool.Annotations, "%s", tool.Name)
			require.True(t, tool.Annotations.ReadOnlyHint, "%s", tool.Name)
		}
	}
	existingTools := []string{
		"auth_get_current_user", "memo_list_memos", "memo_get_memo", "memo_create_memo", "memo_update_memo",
	}
	require.ElementsMatch(t, append(existingTools, newReadTools...), names)

	call := func(name string, args map[string]any) *sdkmcp.CallToolResult {
		t.Helper()
		result, err := mcp.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, err)
		require.False(t, result.IsError, "%v", result.Content)
		return result
	}
	const memoID = "integration-note"
	const originalContent = "Written by an approved integration"
	const taggedContent = originalContent + "\n\n#MCP连接"
	requireTaggedMemo := func(note *sdkmcp.CallToolResult) {
		t.Helper()
		content, ok := note.StructuredContent.(map[string]any)
		require.True(t, ok, "expected structured memo response")
		require.Equal(t, taggedContent, content["content"])
		require.Equal(t, []any{"MCP连接"}, content["tags"])
	}
	createArgs := map[string]any{
		"memoId": memoID, "idempotent": true,
		"body": map[string]any{"content": originalContent},
	}
	requireTaggedMemo(call("memo_create_memo", createArgs))
	requireTaggedMemo(call("memo_create_memo", createArgs))
	requireTaggedMemo(call("memo_get_memo", map[string]any{"memo": memoID}))
	requireTaggedMemo(call("memo_create_memo", map[string]any{
		"memoId": "already-tagged-note",
		"body":   map[string]any{"content": taggedContent},
	}))
	inst.requireMemo(t, browserToken, memoID, taggedContent)
	updated := call("memo_update_memo", map[string]any{
		"memo": memoID,
		"body": map[string]any{"content": "Updated by the integration"},
	})
	content, ok := updated.StructuredContent.(map[string]any)
	require.True(t, ok, "expected structured memo response")
	require.Equal(t, "Updated by the integration", content["content"])
	require.Empty(t, content["tags"])
	require.Equal(t, firstUse, getMCPConnection(t, inst, browserToken, connection.ID, token))

	status, _ = inst.do(t, http.MethodGet, "/api/memos/"+memoID, token, nil)
	require.Equal(t, http.StatusNotFound, status)
	status, _ = inst.connect(t, apiconnect.MemoServiceGetMemoProcedure, token, map[string]string{"name": "memos/" + memoID})
	require.Equal(t, http.StatusUnauthorized, status)
	status, _ = inst.do(t, http.MethodGet, mcpConnectionsPath, token, nil)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	status, _ = mcpDecision(t, inst, token, connection.ID, "revoke")
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	inst.requireMemo(t, browserToken, memoID, "Updated by the integration")

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
}

func TestPersonalMCPNoteDiscovery(t *testing.T) {
	ctx := t.Context()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	ownerToken := inst.signIn(t)
	// In Shanghai, November 21 begins at 16:00 UTC on November 20.
	const query = "查询 A_%"
	const undecidedTask = "\n\n- [ ] 排期"
	for _, memo := range []struct {
		id, timestamp, content string
	}{
		{"before-start", "2026-11-20T15:59:59Z", query + " #项目/开始" + undecidedTask},
		{"at-start", "2026-11-20T16:00:00Z", query + " #项目/发布" + undecidedTask},
		{"at-end", "2026-11-21T15:59:59Z", query + " #项目/发布 #项目/计划 #Team/Release `#项目/代码示例`\n\n- [x] 已处理"},
		{"after-end", "2026-11-21T16:00:00Z", query + " #项目/结束" + undecidedTask},
		{"wrong-tag", "2026-11-21T04:00:00Z", query + " #杂项" + undecidedTask},
		{"wrong-query", "2026-11-21T04:00:00Z", "查询 AXx #项目/发布" + undecidedTask},
		{"untagged", "2026-11-21T04:00:00Z", query + " \\#正文 `#代码`"},
		{"archived", "2026-11-21T04:00:00Z", query + " #项目/归档 #Team/ReleaseArchived" + undecidedTask},
		{"trashed", "2026-11-21T04:00:00Z", query + " #项目/回收 #Team/ReleaseTrashed" + undecidedTask},
	} {
		status, body := inst.connect(t, apiconnect.MemoServiceCreateMemoProcedure, ownerToken, map[string]any{
			"memoId": memo.id, "memo": map[string]string{"content": memo.content, "createTime": memo.timestamp, "visibility": "PRIVATE"},
		})
		require.Equal(t, http.StatusOK, status, "%s: %s", memo.id, body)
	}
	status, body := inst.connect(t, apiconnect.MemoServiceUpdateMemoProcedure, ownerToken, map[string]any{"memo": map[string]string{"name": "memos/archived", "state": "ARCHIVED"}, "updateMask": "state"})
	require.Equal(t, http.StatusOK, status, "%s", body)
	status, body = inst.connect(t, apiconnect.MemoServiceDeleteMemoProcedure, ownerToken, map[string]string{"name": "memos/trashed"})
	require.Equal(t, http.StatusOK, status, "%s", body)
	status, body = inst.connect(t, apiconnect.MemoServiceCreateMemoCommentProcedure, ownerToken, map[string]any{"parent": "memos/at-start", "comment": map[string]string{
		"content": query + " #项目/评论" + undecidedTask, "createTime": "2026-11-21T04:00:00Z", "visibility": "PRIVATE",
	}})
	require.Equal(t, http.StatusOK, status, "%s", body)

	_, connectionToken := createMCPConnection(t, inst, ownerToken, "Read tools fixture", "permanent")
	session, _ := connectMCP(ctx, t, inst, connectionToken)
	call := func(t *testing.T, name string, args map[string]any, response any) {
		t.Helper()
		result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, err)
		require.False(t, result.IsError, "%s: %v", name, result.Content)
		encoded, err := json.Marshal(result.StructuredContent)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(encoded, response))
	}

	t.Run("search", func(t *testing.T) {
		args := map[string]any{
			"query": query, "tag": "#项目", "hasTags": true, "startDate": "2026-11-21", "endDate": "2026-11-21", "orderBy": "create_time asc", "pageSize": 1,
		}
		type result struct {
			Memos         []struct{ Name, CreateTime string }
			NextPageToken string
		}
		var first result
		call(t, "memo_search", args, &first)
		require.Len(t, first.Memos, 1)
		require.Equal(t, "memos/at-start", first.Memos[0].Name)
		require.Equal(t, "2026-11-20T16:00:00Z", first.Memos[0].CreateTime)
		require.NotEmpty(t, first.NextPageToken)
		args["pageToken"] = first.NextPageToken
		var second result
		call(t, "memo_search", args, &second)
		require.Len(t, second.Memos, 1)
		require.Equal(t, "memos/at-end", second.Memos[0].Name)
		require.Equal(t, "2026-11-21T15:59:59Z", second.Memos[0].CreateTime)
		require.Empty(t, second.NextPageToken)

		delete(args, "pageToken")
		args["pageSize"] = 10
		for _, tc := range []struct {
			incomplete bool
			name       string
		}{{true, "memos/at-start"}, {false, "memos/at-end"}} {
			args["hasIncompleteTasks"] = tc.incomplete
			var filtered result
			call(t, "memo_search", args, &filtered)
			require.Len(t, filtered.Memos, 1, "hasIncompleteTasks=%v", tc.incomplete)
			require.Equal(t, tc.name, filtered.Memos[0].Name)
		}
		var untagged result
		call(t, "memo_search", map[string]any{"query": query, "hasTags": false}, &untagged)
		require.Len(t, untagged.Memos, 1)
		require.Equal(t, "memos/untagged", untagged.Memos[0].Name)
	})

	t.Run("batch get", func(t *testing.T) {
		var result struct {
			Memos       []struct{ Name, State, Content string }
			Unavailable []string
		}
		call(t, "memo_batch_get", map[string]any{
			"names": []string{"memos/missing", "memos/at-end", "memos/archived", "memos/trashed", "memos/at-start", "memos/at-end"},
		}, &result)
		require.Equal(t, []string{"memos/missing", "memos/trashed"}, result.Unavailable)
		require.Len(t, result.Memos, 3)
		require.Equal(t, "memos/at-end", result.Memos[0].Name)
		require.Equal(t, "NORMAL", result.Memos[0].State)
		require.Contains(t, result.Memos[0].Content, "已处理")
		require.Equal(t, "memos/archived", result.Memos[1].Name)
		require.Equal(t, "ARCHIVED", result.Memos[1].State)
		require.Equal(t, "memos/at-start", result.Memos[2].Name)
	})

	t.Run("tags", func(t *testing.T) {
		type tag struct {
			Path, Parent string
			MemoCount    int
		}
		type result struct {
			Tags       []tag
			Total      int
			NextOffset *int
		}
		want := []tag{
			{Path: "Team", MemoCount: 1},
			{Path: "Team/Release", Parent: "Team", MemoCount: 1},
			{Path: "杂项", MemoCount: 1},
			{Path: "项目", MemoCount: 5},
			{Path: "项目/发布", Parent: "项目", MemoCount: 3},
			{Path: "项目/开始", Parent: "项目", MemoCount: 1},
			{Path: "项目/结束", Parent: "项目", MemoCount: 1},
			{Path: "项目/计划", Parent: "项目", MemoCount: 1},
		}
		var tree result
		call(t, "tag_tree", nil, &tree)
		require.Equal(t, want, tree.Tags)
		require.Equal(t, len(want), tree.Total)
		require.Nil(t, tree.NextOffset)

		var matches result
		call(t, "tag_search", map[string]any{"query": "#release"}, &matches)
		require.Equal(t, []tag{want[1]}, matches.Tags)
		require.Equal(t, 1, matches.Total)

		var page result
		call(t, "tag_search", map[string]any{"query": "项目/", "pageSize": 2, "offset": 1}, &page)
		require.Equal(t, want[5:7], page.Tags)
		require.Equal(t, 4, page.Total)
		require.NotNil(t, page.NextOffset)
		require.Equal(t, 3, *page.NextOffset)
	})
}

func TestPersonalMCPConnectionsSurviveRestart(t *testing.T) {
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

	status, _ = sendMCPInitialize(t, inst, expiredToken)
	require.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, status)
	require.Equal(t, "expired", getMCPConnection(t, inst, browserToken, expiredConnection.ID, expiredToken).Status)
	status, body = inst.do(t, http.MethodDelete, mcpConnectionsPath+"/"+expiredConnection.ID, browserToken, nil)
	require.Equal(t, http.StatusNoContent, status, "%s", body)
	status, _ = sendMCPInitialize(t, inst, expiredToken)
	require.Equal(t, http.StatusUnauthorized, status)
}
