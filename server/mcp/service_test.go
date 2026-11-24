package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/core/mcpconnection"
	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/ratelimit"
	memosproto "github.com/usememos/memos/proto"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
	"github.com/usememos/memos/store/db"
)

func TestIsAllowedMCPOrigin(t *testing.T) {
	profile := &profile.Profile{InstanceURL: "https://memos.example.com/app"}

	tests := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{name: "empty origin", host: "localhost:5230", origin: "", want: true},
		{name: "same http host", host: "localhost:5230", origin: "http://localhost:5230", want: true},
		{name: "same https host", host: "memos.example.com", origin: "https://memos.example.com", want: true},
		{name: "configured instance URL origin", host: "127.0.0.1:5230", origin: "https://memos.example.com", want: true},
		{name: "explicit default port", host: "127.0.0.1:5230", origin: "https://memos.example.com:443", want: true},
		{name: "non-TLS origin on public host", host: "memos.example.com", origin: "http://memos.example.com", want: false},
		{name: "wrong public port", host: "memos.example.com:8080", origin: "https://memos.example.com:8080", want: false},
		{name: "origin with user info", host: "memos.example.com", origin: "https://user@memos.example.com", want: false},
		{name: "different host", host: "localhost:5230", origin: "https://evil.example.com", want: false},
		{name: "invalid origin", host: "localhost:5230", origin: "not a url", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://"+test.host+"/mcp", nil)
			request.Header.Set("Origin", test.origin)
			require.Equal(t, test.want, isAllowedMCPOrigin(request, profile))
		})
	}
	request := httptest.NewRequest(http.MethodPost, "https://another.example/mcp", nil)
	request.Header.Set("Origin", "https://another.example")
	require.True(t, isAllowedMCPOrigin(request, nil))
}

func TestPersonalMCPRequestBodyLimit(t *testing.T) {
	mux := http.NewServeMux()
	service, err := NewMCPService(&profile.Profile{}, &api.APIService{Store: &store.Store{}})
	require.NoError(t, err)
	service.RegisterRoutes(mux)
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
	request.ContentLength = maxMCPRequestBytes + 1
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}

func TestPersonalToolsShareAccountRateLimit(t *testing.T) {
	limiter := ratelimit.NewMemoryLimiter(ratelimit.Policy{
		ratelimit.ScopeAuthenticated: {Limit: 1, Window: time.Hour},
	})
	service := &MCPService{RateLimiter: limiter}
	success := &sdkmcp.CallToolResult{}
	handler := service.authorizePersonalTools(func(context.Context, string, sdkmcp.Request) (sdkmcp.Result, error) {
		return success, nil
	})
	ctx := auth.SetUserInContext(t.Context(), &store.User{ID: 7}, "")
	ctx = context.WithValue(ctx, connectionPermissionContextKey{}, store.MCPConnectionReadWrite)
	require.True(t, limiter.Consume(ratelimit.ScopeAuthenticated, "7", 1).Allowed)
	for _, tool := range []string{"tag_tree", "memo_search", "memo_create_memo", "unknown"} {
		request := &sdkmcp.CallToolRequest{Params: &sdkmcp.CallToolParamsRaw{Name: tool}}
		result, err := handler(ctx, "tools/call", request)
		require.NoError(t, err)
		require.Equal(t, newToolErrorResult("MCP request rate limit exceeded"), result)
	}
}

func TestNewMCPServiceUsesEmbeddedOpenAPISpec(t *testing.T) {
	t.Chdir(t.TempDir())
	service, err := NewMCPService(&profile.Profile{}, &api.APIService{Store: &store.Store{}})
	require.NoError(t, err)
	require.NotNil(t, service.handler)
}

func TestEmbeddedOpenAPISpecMatchesGeneratedFile(t *testing.T) {
	generated, err := os.ReadFile("../../proto/gen/openapi.yaml")
	require.NoError(t, err)
	require.Equal(t, generated, memosproto.OpenAPIYAML())
}

func TestMCPLoopbackBehindReverseProxy(t *testing.T) {
	mux, token := newMCPTestEndpoint(t)

	initialize, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "memos-test", "version": "1.0.0"},
		},
	})
	require.NoError(t, err)

	// Simulate the proxied deployment: the connection terminates on a loopback
	// address, but the public Host header is a real domain.
	newRequest := func() *http.Request {
		request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(initialize))
		request.Host = "notes.example.com"
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		loopback := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5230}
		ctx := context.WithValue(request.Context(), http.LocalAddrContextKey, loopback)
		return request.WithContext(ctx)
	}

	t.Run("allows non-loopback host with no origin", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, newRequest())
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	})

	t.Run("still rejects a disallowed origin", func(t *testing.T) {
		request := newRequest()
		request.Header.Set("Origin", "https://evil.example.com")
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusForbidden, recorder.Code)
	})
}

func TestMCPStatelessProtocol2026(t *testing.T) {
	mux, token := newMCPTestEndpoint(t)

	meta := map[string]any{
		"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
		"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "memos-test", "version": "1.0.0"},
	}

	discover := postMCPWithHeaders(t, mux, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "server/discover",
		"params":  map[string]any{"_meta": meta},
	}, map[string]string{"Mcp-Method": "server/discover", "Authorization": "Bearer " + token})
	discoverResult, ok := discover["result"].(map[string]any)
	require.True(t, ok, discover)
	require.Contains(t, discoverResult["supportedVersions"], "2026-07-28")
	require.Equal(t, "complete", discoverResult["resultType"])
	capabilities, ok := discoverResult["capabilities"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{}, capabilities["tools"], "tools capability must not advertise listChanged")
	require.NotContains(t, capabilities, "logging", "logging is deprecated and unused")
	require.Positive(t, discoverResult["ttlMs"])

	list := postMCPWithHeaders(t, mux, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{"_meta": meta},
	}, map[string]string{"Mcp-Method": "tools/list", "Authorization": "Bearer " + token})
	listResult, ok := list["result"].(map[string]any)
	require.True(t, ok, list)
	require.Len(t, listResult["tools"], 9)
	require.Equal(t, float64(toolCatalogTTL.Milliseconds()), listResult["ttlMs"])
	require.Equal(t, "private", listResult["cacheScope"])

	call := postMCPWithHeaders(t, mux, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"_meta":     meta,
			"name":      "memo_list_memos",
			"arguments": map[string]any{},
		},
	}, map[string]string{
		"Mcp-Method":    "tools/call",
		"Mcp-Name":      "memo_list_memos",
		"Authorization": "Bearer " + token,
	})
	callResult, ok := call["result"].(map[string]any)
	require.True(t, ok, call)
	require.Equal(t, "complete", callResult["resultType"])
	require.Contains(t, callResult["structuredContent"], "memos")
}

func TestMCPStatelessRejectsSessionMethods(t *testing.T) {
	mux, token := newMCPTestEndpoint(t)

	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		request := httptest.NewRequest(method, "/mcp", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Accept", "application/json, text/event-stream")
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusMethodNotAllowed, recorder.Code, method)
	}
}

func newMCPTestEndpoint(t *testing.T) (http.Handler, string) {
	t.Helper()
	p := &profile.Profile{Personal: true, Data: t.TempDir(), Version: "test-version"}
	require.NoError(t, p.Validate())
	driver, err := db.NewDBDriver(p)
	require.NoError(t, err)
	st := store.New(driver, p)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	require.NoError(t, st.Migrate(t.Context()))
	owner, err := st.CreateUser(t.Context(), &store.User{Username: "mcp-fixture", Role: store.RoleAdmin})
	require.NoError(t, err)
	_, token, err := mcpconnection.Issue(t.Context(), st, owner.ID, "Protocol fixture", "permanent", "", time.Now())
	require.NoError(t, err)
	service, err := NewMCPService(p, &api.APIService{Profile: p, Store: st})
	require.NoError(t, err)
	mux := http.NewServeMux()
	service.RegisterRoutes(mux)
	return mux, token
}

func postMCPWithHeaders(t *testing.T, mux http.Handler, payload map[string]any, headers map[string]string) map[string]any {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if _, hasMeta := payload["params"].(map[string]any)["_meta"]; hasMeta {
		request.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var response map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}
