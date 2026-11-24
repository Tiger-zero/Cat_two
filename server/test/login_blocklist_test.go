package test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/proto/gen/api/apiconnect"
)

const loginBlocklistPath = "/internal/auth/loginBlocklist"

type loginBlocklistPage struct {
	Entries []struct {
		IP        string    `json:"ip"`
		BlockedAt time.Time `json:"blockedAt"`
	} `json:"entries"`
	NextPageToken string `json:"nextPageToken"`
}

func TestPersonalLoginBlocklist(t *testing.T) {
	ctx := t.Context()
	dataDir := t.TempDir()
	opts := instanceOptions{personal: true, rateLimit: true, dataDir: dataDir}
	inst := bootInstance(ctx, t, opts)
	inst.createAdmin(t)
	adminSession := inst.signIn(t)
	post := func(username, password, forwardedFor string) (int, []byte) {
		t.Helper()
		req := inst.connectRequest(t, apiconnect.AuthServiceSignInProcedure, "", map[string]any{
			"passwordCredentials": map[string]string{"username": username, "password": password},
		})
		if forwardedFor != "" {
			req.Header.Set("X-Forwarded-For", forwardedFor)
		}
		response, err := inst.client.Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		if response.StatusCode >= http.StatusBadRequest {
			require.Empty(t, response.Header.Get("Set-Cookie"))
			require.NotContains(t, string(body), "accessToken")
		}
		return response.StatusCode, body
	}
	list := func() loginBlocklistPage {
		t.Helper()
		code, body := inst.do(t, http.MethodGet, loginBlocklistPath, adminSession, nil)
		require.Equal(t, http.StatusOK, code, "%s", body)
		var page loginBlocklistPage
		require.NoError(t, json.Unmarshal(body, &page))
		return page
	}

	for range 3 {
		code, _ := post(testAdminUsername, "wrong", "")
		require.Equal(t, http.StatusBadRequest, code)
	}
	adminSession = inst.signIn(t)
	for range 5 {
		code, _ := post("unknown-account", "wrong", "")
		require.Equal(t, http.StatusBadRequest, code)
	}
	require.Empty(t, list().Entries)
	code, body := post(testAdminUsername, "wrong", "")
	require.Equal(t, http.StatusForbidden, code)
	require.Equal(t, "LOGIN_IP_BLOCKED", requireConnectErrorInfo(t, body, "permission_denied").Reason)
	entries := list().Entries
	require.Len(t, entries, 1)
	require.Equal(t, "127.0.0.1", entries[0].IP)
	require.WithinDuration(t, time.Now(), entries[0].BlockedAt, time.Minute)
	code, _ = inst.do(t, http.MethodGet, loginBlocklistPath, "", nil)
	require.Equal(t, http.StatusUnauthorized, code)
	_, mcpToken := createMCPConnection(t, inst, adminSession, "Login management authorization fixture", "")
	code, _ = inst.do(t, http.MethodDelete, loginBlocklistPath+"/127.0.0.1", mcpToken, nil)
	require.Equal(t, http.StatusUnauthorized, code)

	inst.shutdown(ctx)
	inst = bootInstance(ctx, t, opts)
	code, body = post(testAdminUsername, testAdminPassword, "203.0.113.5")
	require.Equal(t, http.StatusForbidden, code)
	require.Contains(t, string(body), `"code":"permission_denied"`)
	require.Equal(t, entries, list().Entries, "a restart must preserve the ban")
	code, _ = inst.do(t, http.MethodDelete, loginBlocklistPath+"/127.0.0.1", adminSession, nil)
	require.Equal(t, http.StatusNoContent, code)
	require.Empty(t, list().Entries)
	inst.signIn(t)
}

func TestLoginBlocklistPagination(t *testing.T) {
	ctx := t.Context()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	adminSession := inst.signIn(t)
	for n := range 100 {
		_, blocked, err := inst.server.Store.RecordLoginIPFailure(ctx, fmt.Sprintf("192.0.2.%d", n), 1)
		require.NoError(t, err)
		require.True(t, blocked)
	}
	const ipv6 = "2001:db8::1"
	_, _, err := inst.server.Store.RecordLoginIPFailure(ctx, ipv6, 1)
	require.NoError(t, err)
	code, body := inst.do(t, http.MethodGet, loginBlocklistPath, adminSession, nil)
	require.Equal(t, http.StatusOK, code)
	var first, last loginBlocklistPage
	require.NoError(t, json.Unmarshal(body, &first))
	require.Len(t, first.Entries, 100)
	require.NotEmpty(t, first.NextPageToken)
	code, body = inst.do(t, http.MethodGet, loginBlocklistPath+"?pageToken="+url.QueryEscape(first.NextPageToken), adminSession, nil)
	require.Equal(t, http.StatusOK, code)
	require.NoError(t, json.Unmarshal(body, &last))
	require.Len(t, last.Entries, 1)
	require.Equal(t, ipv6, last.Entries[0].IP)
	require.Empty(t, last.NextPageToken)
	code, _ = inst.do(t, http.MethodDelete, loginBlocklistPath+"/"+url.PathEscape(ipv6), adminSession, nil)
	require.Equal(t, http.StatusNoContent, code)
	blocked, err := inst.server.Store.IsLoginIPBlocked(ctx, ipv6)
	require.NoError(t, err)
	require.False(t, blocked)
}
