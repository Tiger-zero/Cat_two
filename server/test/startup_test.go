// Package test contains black-box startup smoke tests for the full server.
//
// Every other server test constructs api.APIService directly, which skips
// server.NewServer entirely. That leaves route registration, Connect RPC
// wiring, MCP/fileserver/frontend mounting, CORS and the secret bootstrap
// covered only by the Docker release script. These tests boot the real server
// the same way cmd/memos/main.go does so a wiring regression fails in CI.
package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	// sqlite driver.
	_ "modernc.org/sqlite"

	"github.com/usememos/memos/core/configuration"
	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/version"
	"github.com/usememos/memos/proto/gen/api/apiconnect"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server"
	"github.com/usememos/memos/store"
	"github.com/usememos/memos/store/db"
)

const (
	testAdminUsername = "startup-admin"
	testAdminPassword = "startup-password"
)

// instanceOptions configures a single server boot.
type instanceOptions struct {
	// personal uses the policy enabled by the Cat_two executable.
	personal bool
	// instanceURL is the canonical external address used by browser origin checks.
	instanceURL string
	// rateLimit enables the request rate limiter, which the other tests leave
	// off so that their request volume never trips a budget.
	rateLimit bool
	// dataDir reuses the same data directory for restart tests.
	dataDir string
	// configDir is the declarative configuration directory applied at
	// startup; empty means a directory that does not exist.
	configDir string
}

// instance is a booted server plus the HTTP plumbing needed to talk to it.
type instance struct {
	server  *server.Server
	profile *profile.Profile
	baseURL string
	client  *http.Client
}

// bootInstance starts a full server following the same sequence as
// cmd/memos/main.go: validate profile, open SQLite, initialize the schema,
// apply declarative configuration, construct the server, then listen.
//
// Keep this in lockstep with main.go. The value of these tests comes from
// exercising the real startup path rather than a hand-assembled subset of it.
func bootInstance(ctx context.Context, t *testing.T, opts instanceOptions) *instance {
	t.Helper()

	dataDir := opts.dataDir
	if dataDir == "" {
		dataDir = t.TempDir()
	}

	instanceProfile := &profile.Profile{
		Personal:    opts.personal,
		Addr:        "127.0.0.1",
		Port:        unusedPort(t),
		Data:        dataDir,
		Driver:      "sqlite",
		InstanceURL: opts.instanceURL,
		RateLimit:   opts.rateLimit,
		Version:     version.GetCurrentVersion(),
		Commit:      version.Commit,
	}
	require.NoError(t, instanceProfile.Validate(), "profile should validate")

	dbDriver, err := db.NewDBDriver(instanceProfile)
	require.NoError(t, err, "should open database driver")

	storeInstance := store.New(dbDriver, instanceProfile)
	require.NoError(t, storeInstance.InitializeSchema(ctx), "should migrate database")

	configDir := opts.configDir
	if configDir == "" {
		configDir = filepath.Join(dataDir, "no-configuration")
	}
	desired, err := configuration.Load(configDir, configuration.OSResolver())
	require.NoError(t, err, "should load the configuration")
	plan, _, err := configuration.Apply(ctx, desired, storeInstance, configuration.ApplyMetadata{AppliedBy: "startup test", Now: time.Now()})
	require.NoError(t, err, "should apply the configuration")
	require.True(t, plan.Valid(), "the configuration should be applicable: %v", plan.Message.Errors)
	require.NoError(t, storeInstance.SweepMemoBodies(ctx), "should check Markdown body storage")

	s, err := server.NewServer(ctx, instanceProfile, storeInstance)
	require.NoError(t, err, "should construct server")
	require.NoError(t, s.Start(), "should start server")

	inst := &instance{
		server:  s,
		profile: instanceProfile,
		baseURL: fmt.Sprintf("http://127.0.0.1:%d", instanceProfile.Port),
		client:  &http.Client{Timeout: 10 * time.Second},
	}
	t.Cleanup(func() {
		inst.shutdown(context.Background())
	})
	inst.waitUntilReady(t)
	return inst
}

// shutdown stops the server. Server.Shutdown also closes the store, so a
// subsequent boot against the same data directory must build a new driver,
// which is what bootInstance does and what a real restart does.
func (i *instance) shutdown(ctx context.Context) {
	if i.server == nil {
		return
	}
	i.server.Shutdown(ctx)
	i.server = nil
}

func (i *instance) waitUntilReady(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		resp, err := i.client.Get(i.baseURL + "/healthz")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 30*time.Second, 100*time.Millisecond, "server should become ready")
}

// do issues a native HTTP request against the instance.
func (i *instance) do(t *testing.T, method, path, token string, body any) (int, []byte) {
	t.Helper()
	return i.readResponse(t, i.newRequest(t, method, path, token, body))
}

// connect sends a real unary Connect JSON POST to a generated procedure path.
func (i *instance) connect(t *testing.T, procedure, token string, body any) (int, []byte) {
	t.Helper()
	return i.readResponse(t, i.connectRequest(t, procedure, token, body))
}

func (i *instance) connectRequest(t *testing.T, procedure, token string, body any) *http.Request {
	t.Helper()
	if body == nil {
		body = struct{}{}
	}
	req := i.newRequest(t, http.MethodPost, procedure, token, body)
	req.Header.Set("Connect-Protocol-Version", "1")
	return req
}

func (i *instance) newRequest(t *testing.T, method, path, token string, body any) *http.Request {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		require.NoError(t, err)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, i.baseURL+path, bytes.NewReader(encoded))
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

func (i *instance) readResponse(t *testing.T, req *http.Request) (int, []byte) {
	t.Helper()
	resp, err := i.client.Do(req)
	require.NoError(t, err, "%s %s should not fail at the transport level", req.Method, req.URL.Path)
	defer resp.Body.Close()
	payload := new(bytes.Buffer)
	_, err = payload.ReadFrom(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, payload.Bytes()
}

// createAdmin registers the first user, which the service promotes to admin.
func (i *instance) createAdmin(t *testing.T) {
	t.Helper()

	status, body := i.connect(t, apiconnect.UserServiceCreateUserProcedure, "", map[string]any{"user": map[string]any{
		"username": testAdminUsername,
		"password": testAdminPassword,
		"email":    "startup-admin@example.test",
	}})
	require.Equal(t, http.StatusOK, status, "creating the first user should succeed: %s", body)

	var created struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	require.NoError(t, json.Unmarshal(body, &created))
	require.Equal(t, testAdminUsername, created.Username)
	require.Equal(t, "ADMIN", created.Role, "the first user should be an admin")
}

// signIn authenticates as the admin created by createAdmin.
func (i *instance) signIn(t *testing.T) string {
	t.Helper()
	return i.signInUser(t, testAdminUsername, testAdminPassword)
}

func (i *instance) signInUser(t *testing.T, username, password string) string {
	t.Helper()

	status, body := i.connect(t, apiconnect.AuthServiceSignInProcedure, "", map[string]any{
		"passwordCredentials": map[string]any{
			"username": username,
			"password": password,
		},
	})
	require.Equal(t, http.StatusOK, status, "sign-in should succeed: %s", body)

	var signedIn struct {
		AccessToken string `json:"accessToken"`
	}
	require.NoError(t, json.Unmarshal(body, &signedIn))
	require.NotEmpty(t, signedIn.AccessToken, "sign-in should return an access token")
	return signedIn.AccessToken
}

// createMemo writes a memo with a caller-supplied id so it can be re-read by name.
func (i *instance) createMemo(t *testing.T, token, memoID, content string) {
	t.Helper()

	status, body := i.connect(t, apiconnect.MemoServiceCreateMemoProcedure, token, map[string]any{
		"memoId": memoID, "memo": map[string]any{"content": content, "visibility": "PRIVATE"},
	})
	require.Equal(t, http.StatusOK, status, "creating a memo should succeed: %s", body)

	var created struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	require.NoError(t, json.Unmarshal(body, &created))
	require.Equal(t, "memos/"+memoID, created.Name)
	require.Equal(t, content, created.Content)
}

// requireMemo asserts a memo is readable and has the expected content.
func (i *instance) requireMemo(t *testing.T, token, memoID, content string) {
	t.Helper()

	status, body := i.connect(t, apiconnect.MemoServiceGetMemoProcedure, token, map[string]string{"name": "memos/" + memoID})
	require.Equal(t, http.StatusOK, status, "reading memo %s should succeed: %s", memoID, body)

	var fetched struct {
		Content string `json:"content"`
	}
	require.NoError(t, json.Unmarshal(body, &fetched))
	require.Equal(t, content, fetched.Content)
}

func unusedPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

// TestHealthzReportsDatabaseOutage verifies /healthz checks the database
// instead of always answering ready.
func TestHealthzReportsDatabaseOutage(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{instanceURL: "http://localhost"})

	status, _ := inst.do(t, http.MethodGet, "/healthz", "", nil)
	require.Equal(t, http.StatusOK, status)

	require.NoError(t, inst.server.Store.GetDriver().GetDB().Close())
	status, body := inst.do(t, http.MethodGet, "/healthz", "", nil)
	require.Equal(t, http.StatusServiceUnavailable, status)
	require.Equal(t, "Database unavailable.", string(body))
}

// TestStartupServesEveryRegisteredRouter boots a fresh instance and checks that
// each router mounted by server.NewServer actually answers. A registration
// order regression or a gateway conflict shows up here as a 404.
func TestStartupServesEveryRegisteredRouter(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{instanceURL: "http://localhost"})

	t.Run("healthz", func(t *testing.T) {
		status, body := inst.do(t, http.MethodGet, "/healthz", "", nil)
		require.Equal(t, http.StatusOK, status)
		require.Equal(t, "Service ready.", string(body))
	})

	t.Run("frontend", func(t *testing.T) {
		// CI only has the placeholder dist/index.html, so assert the route is
		// mounted and serving HTML rather than asserting on built markup.
		status, body := inst.do(t, http.MethodGet, "/", "", nil)
		require.Equal(t, http.StatusOK, status)
		require.Contains(t, strings.ToLower(string(body)), "<!doctype html>")
	})

	t.Run("Connect RPC", func(t *testing.T) {
		status, body := inst.connect(t, apiconnect.InstanceServiceGetInstanceProfileProcedure, "", nil)
		require.Equal(t, http.StatusOK, status, "instance profile should be public: %s", body)
		var profile struct {
			NeedsSetup bool
		}
		require.NoError(t, json.Unmarshal(body, &profile))
		require.True(t, profile.NeedsSetup)
	})

	t.Run("mcp", func(t *testing.T) {
		// A bare POST is enough to prove the handler is mounted; the MCP
		// protocol itself is covered by server/mcp tests.
		status, _ := inst.do(t, http.MethodPost, "/mcp", "", map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/list",
		})
		require.NotEqual(t, http.StatusNotFound, status, "mcp route should be mounted")
	})

	t.Run("oauth discovery is not advertised", func(t *testing.T) {
		// MCP clients probe RFC 9728 metadata before trusting the endpoint.
		// The MCP server authenticates with connection credentials, so the
		// well-known paths must 404 instead of falling back to the SPA.
		for _, requestPath := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp"} {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, inst.baseURL+requestPath, nil)
			require.NoError(t, err)
			resp, err := inst.client.Do(req)
			require.NoError(t, err)
			resp.Body.Close()

			require.Equal(t, http.StatusNotFound, resp.StatusCode, requestPath)
			require.NotContains(t, resp.Header.Get("Content-Type"), "text/html", requestPath)
		}
	})
}

func TestLegacyRESTRoutesAreRetired(t *testing.T) {
	inst := bootInstance(t.Context(), t, instanceOptions{personal: true})
	inst.createAdmin(t)
	session := inst.signIn(t)

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api"},
		{http.MethodGet, "/api/"},
		{http.MethodGet, "/api/instance/profile"},
		{http.MethodGet, "/api/v1/instance/profile"},
		{http.MethodPost, "/api/auth/signin"},
		{http.MethodPost, "/api/memos"},
		{http.MethodPost, "/api/users/" + testAdminUsername + "/personalAccessTokens"},
		{http.MethodGet, "/api/mcpConnections"},
		{http.MethodGet, "/api/auth/loginBlocklist"},
		{http.MethodGet, "/api/sse"},
		{http.MethodPost, "/memos.api.v1.AuthService/GetCurrentUser"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			for _, credential := range []string{"", session} {
				req := inst.newRequest(t, route.method, route.path, credential, nil)
				resp, err := inst.client.Do(req)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				require.Equal(t, http.StatusNotFound, resp.StatusCode)
				require.NotContains(t, resp.Header.Get("Content-Type"), "text/html")
			}
		})
	}
}

// TestStartupFreshInstallRoundTrip covers the new-install path end to end:
// register the first user, authenticate, then write and read a memo through
// Connect RPC.
func TestStartupFreshInstallRoundTrip(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{instanceURL: "http://localhost"})

	inst.createAdmin(t)
	token := inst.signIn(t)
	inst.createMemo(t, token, "startup-fresh", "fresh install sentinel")
	inst.requireMemo(t, token, "startup-fresh", "fresh install sentinel")

	viewPath := "/api/users/" + testAdminUsername + "/views"
	for _, retired := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, viewPath},
		{http.MethodPost, viewPath},
		{http.MethodPost, "/memos.api.UserService/ListMemoViews"},
	} {
		status, body := inst.do(t, retired.method, retired.path, token, nil)
		require.Equal(t, http.StatusNotFound, status, "%s %s: %s", retired.method, retired.path, body)
	}
}

func TestStartupRetiredTranscriptionRoutes(t *testing.T) {
	inst := bootInstance(t.Context(), t, instanceOptions{personal: true})

	inst.createAdmin(t)
	token := inst.signIn(t)
	for _, path := range []string{"/api/ai:transcribe", "/memos.api.AIService/Transcribe"} {
		status, body := inst.do(t, http.MethodPost, path, token, map[string]any{
			"audio": map[string]string{"content": "YXVkaW8=", "filename": "note.wav", "contentType": "audio/wav"},
		})
		require.Equal(t, http.StatusNotFound, status, "%s: %s", path, body)
	}
}

// TestStartupRestartPreservesData boots, writes data, shuts down, then boots a
// second time against the same data directory. It checks that schema
// initialization and startup preserve notes and administrator credentials.
func TestStartupRestartPreservesData(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()

	first := bootInstance(ctx, t, instanceOptions{
		instanceURL: "http://localhost",
		dataDir:     dataDir,
	})
	first.createAdmin(t)
	token := first.signIn(t)
	first.createMemo(t, token, "startup-restart", "written before restart")
	firstPort := first.profile.Port
	first.shutdown(ctx)

	second := bootInstance(ctx, t, instanceOptions{
		instanceURL: "http://localhost",
		dataDir:     dataDir,
	})
	require.NotEqual(t, firstPort, second.profile.Port, "the second boot should bind a new port")

	// Tokens are signed with the instance secret, which must survive a restart.
	restartToken := second.signIn(t)
	second.requireMemo(t, restartToken, "startup-restart", "written before restart")
	second.createMemo(t, restartToken, "startup-after-restart", "written after restart")
	second.requireMemo(t, restartToken, "startup-after-restart", "written after restart")
}

// TestStartupPrivateInstance verifies a private instance still exposes the auth
// bootstrap surface and refuses anonymous callers on non-bootstrap procedures.
func TestStartupPrivateInstance(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{instanceURL: ""})

	accessSetting, err := inst.server.Store.GetInstanceAccessSetting(ctx)
	require.NoError(t, err)
	require.Equal(t, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE, accessSetting.AccessMode)

	// Bootstrap methods stay reachable so the sign-in page can render.
	status, body := inst.connect(t, apiconnect.InstanceServiceGetInstanceProfileProcedure, "", nil)
	require.Equal(t, http.StatusOK, status, "instance profile is an auth bootstrap method: %s", body)

	// Protected procedures are refused for anonymous callers.
	status, _ = inst.connect(t, apiconnect.UserServiceListUsersProcedure, "", nil)
	require.Equal(t, http.StatusUnauthorized, status, "anonymous ListUsers should be refused")

	// Memo content requires authentication on Connect.
	status, _ = inst.connect(t, apiconnect.MemoServiceListMemosProcedure, "", nil)
	require.Equal(t, http.StatusUnauthorized, status,
		"anonymous ListMemos over Connect should be refused on a private instance")

	// Authenticated access is never affected by private mode.
	inst.createAdmin(t)
	token := inst.signIn(t)
	inst.createMemo(t, token, "startup-private", "private instance sentinel")
	inst.requireMemo(t, token, "startup-private", "private instance sentinel")
}

// TestStartupPrivateInstanceConnectPolicy checks that historical PUBLIC notes
// cannot be read anonymously through the live Connect router.
func TestStartupPrivateInstanceConnectPolicy(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{instanceURL: ""})

	inst.createAdmin(t)
	token := inst.signIn(t)
	owner, err := inst.server.Store.GetUser(ctx, &store.FindUser{Username: new(testAdminUsername)})
	require.NoError(t, err)
	_, err = inst.server.Store.CreateMemo(ctx, &store.Memo{
		UID: "startup-private-public", CreatorID: owner.ID, Content: "legacy public note", Visibility: store.Public,
	})
	require.NoError(t, err)
	inst.requireMemo(t, token, "startup-private-public", "legacy public note")

	status, _ := inst.connect(t, apiconnect.MemoServiceListMemosProcedure, "", nil)
	require.Equal(t, http.StatusUnauthorized, status,
		"anonymous ListMemos through Connect should be refused on a private instance")

	status, _ = inst.connect(t, apiconnect.MemoServiceGetMemoProcedure, "", map[string]string{"name": "memos/startup-private-public"})
	require.Equal(t, http.StatusUnauthorized, status,
		"anonymous GetMemo through Connect should be refused on a private instance")
}

// TestStartupConnectOmitsNullMessageFields checks the live JSON codec for
// attachments without motion media.
func TestStartupConnectOmitsNullMessageFields(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{instanceURL: "http://localhost"})

	inst.createAdmin(t)
	token := inst.signIn(t)

	// A 1x1 GIF: small enough to inline, real enough for the type sniffing the
	// attachment service runs on upload.
	const onePixelGIF = "R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7"
	status, body := inst.connect(t, apiconnect.AttachmentServiceCreateAttachmentProcedure, token, map[string]any{"attachment": map[string]any{
		"filename": "pixel.gif",
		"type":     "image/gif",
		"content":  onePixelGIF,
	}})
	require.Equal(t, http.StatusOK, status, "creating an attachment should succeed: %s", body)

	var created struct {
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(body, &created))
	require.NotEmpty(t, created.Name, "the created attachment should carry a name: %s", body)

	status, body = inst.connect(t, apiconnect.AttachmentServiceGetAttachmentProcedure, token, map[string]string{"name": created.Name})
	require.Equal(t, http.StatusOK, status, "reading the attachment should succeed: %s", body)

	fetched := map[string]any{}
	require.NoError(t, json.Unmarshal(body, &fetched))
	require.NotContains(t, fetched, "motionMedia", "an unset message field must be omitted, not null: %s", body)
	require.Equal(t, "pixel.gif", fetched["filename"])
	require.Equal(t, "image/gif", fetched["type"])
}

// TestStartupAppliesConfiguration boots a headless instance from files alone:
// the administrator signs in without anyone having opened the browser, and the
// configured settings are what the API serves.
func TestStartupAppliesConfiguration(t *testing.T) {
	ctx := context.Background()
	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "instance.json"), []byte(`{
		"instanceSettings": [
			{"name": "instance/settings/GENERAL", "generalSetting": {"customProfile": {"title": "Configured Memos"}}}
		],
		"users": [{"name": "users/`+testAdminUsername+`", "username": "`+testAdminUsername+`", "role": "ADMIN", "password": "${env:MEMOS_TEST_ADMIN_PASSWORD}"}]
	}`), 0o600))
	t.Setenv("MEMOS_TEST_ADMIN_PASSWORD", testAdminPassword)
	inst := bootInstance(ctx, t, instanceOptions{configDir: configDir, personal: true})

	token := inst.signIn(t)
	status, body := inst.connect(t, apiconnect.InstanceServiceGetInstanceSettingProcedure, token, map[string]string{"name": "instance/settings/GENERAL"})
	require.Equal(t, http.StatusOK, status, "%s", body)
	require.Contains(t, string(body), "Configured Memos")

	status, body = inst.connect(t, apiconnect.InstanceServiceGetConfigurationStatusProcedure, token, nil)
	require.Equal(t, http.StatusNotImplemented, status, "%s", body)
	state, err := inst.server.Store.GetConfigurationState(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), state.Serial)
	require.Len(t, state.Resources, 2)
	require.Equal(t, "instance/settings/GENERAL", state.Resources[0].Name)
	require.Equal(t, "users/"+testAdminUsername, state.Resources[1].Name)
}
