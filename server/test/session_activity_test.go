package test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/usememos/memos/proto/gen/api/apiconnect"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

func TestPersonalWebSessionExpiresWithoutEdits(t *testing.T) {
	ctx := t.Context()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	owner, err := inst.server.Store.GetUser(ctx, &store.FindUser{Username: new(testAdminUsername)})
	require.NoError(t, err)
	require.NotNil(t, owner)

	const activityPath = "/internal/session/activity"
	const idleHeader = "X-Memos-Session-Idle-Expires-At"
	type result struct {
		status  int
		body    string
		headers http.Header
		cookies []*http.Cookie
	}
	request := func(method, path, token, body, origin string, cookie *http.Cookie) result {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, inst.baseURL+path, strings.NewReader(body))
		require.NoError(t, err)
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if strings.HasPrefix(path, "/memos.api.") {
			req.Header.Set("Connect-Protocol-Version", "1")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		resp, err := inst.client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		payload, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return result{status: resp.StatusCode, body: string(payload), headers: resp.Header, cookies: resp.Cookies()}
	}
	type tokenResponse struct {
		AccessToken string `json:"accessToken"`
	}
	credentials, err := json.Marshal(map[string]any{
		"passwordCredentials": map[string]string{"username": testAdminUsername, "password": testAdminPassword},
	})
	require.NoError(t, err)
	signIn := request(http.MethodPost, apiconnect.AuthServiceSignInProcedure, "", string(credentials), inst.baseURL, nil)
	require.Equal(t, http.StatusOK, signIn.status, "%s", signIn.body)
	require.Len(t, signIn.cookies, 1)
	var signedIn tokenResponse
	require.NoError(t, json.Unmarshal([]byte(signIn.body), &signedIn))
	initialDeadline, err := time.Parse(time.RFC3339Nano, signIn.headers.Get(idleHeader))
	require.NoError(t, err)
	stored, err := inst.server.Store.GetUserRefreshTokens(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, stored[0].GetCreatedAt().AsTime(), stored[0].GetLastEditAt().AsTime())
	require.Equal(t, stored[0].GetLastEditAt().AsTime().Add(store.UserSessionIdleTimeout), initialDeadline)

	me := request(http.MethodPost, apiconnect.AuthServiceGetCurrentUserProcedure, signedIn.AccessToken, "{}", "", nil)
	require.Equal(t, http.StatusOK, me.status, "%s", me.body)
	require.Equal(t, signIn.headers.Get(idleHeader), me.headers.Get(idleHeader))
	refresh := request(http.MethodPost, apiconnect.AuthServiceRefreshTokenProcedure, "", "{}", inst.baseURL, signIn.cookies[0])
	require.Equal(t, http.StatusOK, refresh.status, "%s", refresh.body)
	require.Equal(t, signIn.headers.Get(idleHeader), refresh.headers.Get(idleHeader))
	require.Len(t, refresh.cookies, 1)
	require.NotEqual(t, signIn.cookies[0].Value, refresh.cookies[0].Value)

	_, mcpToken := createMCPConnection(t, inst, signedIn.AccessToken, "Idle session isolation fixture", "")
	mcpActivity := request(http.MethodPost, activityPath, mcpToken, "", inst.baseURL, nil)
	require.Equal(t, http.StatusUnauthorized, mcpActivity.status)
	crossOrigin := request(http.MethodPost, activityPath, signedIn.AccessToken, "", "https://other.example.test", nil)
	require.Equal(t, http.StatusForbidden, crossOrigin.status)
	noOrigin := request(http.MethodPost, activityPath, signedIn.AccessToken, "", "", nil)
	require.Equal(t, http.StatusForbidden, noOrigin.status)

	activity := request(http.MethodPost, activityPath, signedIn.AccessToken, "", inst.baseURL, nil)
	require.Equal(t, http.StatusOK, activity.status, "%s", activity.body)
	require.Contains(t, activity.headers.Get("Cache-Control"), "no-store")
	var extended struct{ IdleExpiresAt time.Time }
	require.NoError(t, json.Unmarshal([]byte(activity.body), &extended))
	require.True(t, extended.IdleExpiresAt.After(initialDeadline))
	me = request(http.MethodPost, apiconnect.AuthServiceGetCurrentUserProcedure, signedIn.AccessToken, "{}", "", nil)
	require.Equal(t, extended.IdleExpiresAt.UTC().Format(time.RFC3339Nano), me.headers.Get(idleHeader))

	stored, err = inst.server.Store.GetUserRefreshTokens(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	stored[0].LastEditAt = timestamppb.New(time.Now().Add(-store.UserSessionIdleTimeout - time.Minute))
	_, err = inst.server.Store.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: owner.ID, Key: storepb.UserSetting_REFRESH_TOKENS,
		Value: &storepb.UserSetting_RefreshTokens{RefreshTokens: &storepb.RefreshTokensUserSetting{RefreshTokens: stored}},
	})
	require.NoError(t, err)
	me = request(http.MethodPost, apiconnect.AuthServiceGetCurrentUserProcedure, signedIn.AccessToken, "{}", "", nil)
	require.Equal(t, http.StatusUnauthorized, me.status)
	staleRefresh := request(http.MethodPost, apiconnect.AuthServiceRefreshTokenProcedure, "", "{}", inst.baseURL, refresh.cookies[0])
	require.Equal(t, http.StatusUnauthorized, staleRefresh.status)
	require.Contains(t, staleRefresh.body, "session idle expired")
	require.Empty(t, staleRefresh.cookies)
	staleActivity := request(http.MethodPost, activityPath, signedIn.AccessToken, "", inst.baseURL, nil)
	require.Equal(t, http.StatusUnauthorized, staleActivity.status)
	require.Contains(t, staleActivity.body, "session idle expired")
	mcpRead := request(http.MethodPost, apiconnect.AuthServiceGetCurrentUserProcedure, mcpToken, "{}", "", nil)
	require.Equal(t, http.StatusUnauthorized, mcpRead.status)
	require.Empty(t, mcpRead.headers.Get(idleHeader))
}
