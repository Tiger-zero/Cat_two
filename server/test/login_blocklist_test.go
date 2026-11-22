package test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

const loginBlocklistPath = "/api/auth/loginBlocklist"

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
	post := func(path, username, password string, forwardedFor string) (int, []byte) {
		t.Helper()
		payload, err := json.Marshal(map[string]any{"passwordCredentials": map[string]string{"username": username, "password": password}})
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, inst.baseURL+path, bytes.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Connect-Protocol-Version", "1")
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
		code, _ := post("/api/auth/signin", testAdminUsername, "wrong", "")
		require.Equal(t, http.StatusBadRequest, code)
	}
	adminSession = inst.signIn(t)
	const connectPath = "/memos.api.AuthService/SignIn"
	for range 5 {
		code, _ := post(connectPath, "unknown-account", "wrong", "")
		require.Equal(t, http.StatusBadRequest, code)
	}
	require.Empty(t, list().Entries)
	code, body := post("/api/auth/signin", testAdminUsername, "wrong", "")
	require.Equal(t, http.StatusForbidden, code)
	require.Contains(t, string(body), "LOGIN_IP_BLOCKED")
	entries := list().Entries
	require.Len(t, entries, 1)
	require.Equal(t, "127.0.0.1", entries[0].IP)
	require.WithinDuration(t, time.Now(), entries[0].BlockedAt, time.Minute)
	code, _ = inst.do(t, http.MethodGet, loginBlocklistPath, "", nil)
	require.Equal(t, http.StatusUnauthorized, code)
	code, body = inst.do(t, http.MethodPost, "/api/users/"+testAdminUsername+"/personalAccessTokens", adminSession, map[string]string{"description": "test REST integration"})
	require.Equal(t, http.StatusOK, code, "%s", body)
	var pat struct{ Token string }
	require.NoError(t, json.Unmarshal(body, &pat))
	require.NotEmpty(t, pat.Token)
	code, _ = inst.do(t, http.MethodDelete, loginBlocklistPath+"/127.0.0.1", pat.Token, nil)
	require.Equal(t, http.StatusForbidden, code)

	inst.shutdown(ctx)
	inst = bootInstance(ctx, t, opts)
	code, body = post(connectPath, testAdminUsername, testAdminPassword, "203.0.113.5")
	require.Equal(t, http.StatusForbidden, code)
	require.Contains(t, string(body), `"code":"permission_denied"`)
	require.Equal(t, entries, list().Entries, "a restart must preserve the ban")
	code, _ = inst.do(t, http.MethodDelete, loginBlocklistPath+"/127.0.0.1", adminSession, nil)
	require.Equal(t, http.StatusNoContent, code)
	require.Empty(t, list().Entries)
	inst.signIn(t)
}

func TestLoginBlocklistPaginationAndAdminAuthorization(t *testing.T) {
	ctx := t.Context()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	adminSession := inst.signIn(t)
	legacyUser, err := inst.server.Store.CreateUser(ctx, &store.User{Username: "legacy-viewer", Role: store.RoleUser})
	require.NoError(t, err)
	const sessionID = "legacy-viewer-session"
	require.NoError(t, inst.server.Store.AddUserRefreshToken(ctx, legacyUser.ID, &storepb.RefreshTokensUserSetting_RefreshToken{TokenId: "legacy-viewer-credential", SessionId: sessionID}))
	legacyToken, _, err := auth.GenerateSessionAccessToken(legacyUser, sessionID, "", []byte(inst.server.Secret))
	require.NoError(t, err)
	code, _ := inst.do(t, http.MethodGet, loginBlocklistPath, legacyToken, nil)
	require.Equal(t, http.StatusForbidden, code)

	for n := range 100 {
		_, blocked, err := inst.server.Store.RecordLoginIPFailure(ctx, fmt.Sprintf("192.0.2.%d", n), 1)
		require.NoError(t, err)
		require.True(t, blocked)
	}
	const ipv6 = "2001:db8::1"
	_, _, err = inst.server.Store.RecordLoginIPFailure(ctx, ipv6, 1)
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
