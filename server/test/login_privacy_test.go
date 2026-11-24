package test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/proto/gen/api/apiconnect"
	"github.com/usememos/memos/store"
)

func TestPersonalLoginAccountPrivacy(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	const email = "startup-admin@example.test"
	status, created := inst.connect(t, apiconnect.UserServiceCreateUserProcedure, "", map[string]any{
		"user": map[string]string{"username": testAdminUsername, "password": testAdminPassword, "email": email},
	})
	require.Equal(t, http.StatusOK, status, "%s", created)
	username := testAdminUsername
	user, err := inst.server.Store.GetUser(ctx, &store.FindUser{Username: &username})
	require.NoError(t, err)
	require.NotNil(t, user)
	require.NotEmpty(t, user.PasswordHash)
	assertNoSecrets := func(t *testing.T, body []byte) {
		t.Helper()
		require.NotContains(t, string(body), testAdminPassword)
		require.False(t, bytes.Contains(body, []byte(user.PasswordHash)), "response must not include the stored password hash")
		require.False(t, bytes.Contains(body, []byte(inst.server.Secret)), "response must not include the instance signing key")
	}
	assertNoSecrets(t, created)

	assertPublicResponse := func(status int, body []byte) {
		t.Helper()
		require.Equal(t, http.StatusOK, status, "%s", body)
		require.NotContains(t, string(body), testAdminUsername)
		require.NotContains(t, string(body), email)
		assertNoSecrets(t, body)
	}
	for _, path := range []string{"/auth", "/auth/admin"} {
		assertPublicResponse(inst.do(t, http.MethodGet, path, "", nil))
	}
	assertPublicResponse(inst.connect(t, apiconnect.InstanceServiceGetInstanceProfileProcedure, "", nil))
	for _, name := range []string{"instance/settings/BASIC", "instance/settings/GENERAL"} {
		assertPublicResponse(inst.connect(t, apiconnect.InstanceServiceGetInstanceSettingProcedure, "", map[string]string{"name": name}))
	}

	assertProtectedResponse := func(status int, body []byte) {
		t.Helper()
		require.Equal(t, http.StatusUnauthorized, status, "%s", body)
		require.NotContains(t, string(body), email)
		assertNoSecrets(t, body)
	}
	assertProtectedResponse(inst.connect(t, apiconnect.UserServiceGetUserProcedure, "", map[string]string{"name": "users/" + username}))
	assertProtectedResponse(inst.connect(t, apiconnect.AuthServiceGetCurrentUserProcedure, "", nil))
	assertProtectedResponse(inst.do(t, http.MethodGet, "/file/users/"+username+"/avatar", "", nil))

	signIn := func(username, password string) (int, []byte) {
		return inst.connect(t, apiconnect.AuthServiceSignInProcedure, "", map[string]any{
			"passwordCredentials": map[string]string{"username": username, "password": password},
		})
	}
	status, body := signIn(testAdminUsername, testAdminPassword)
	require.Equal(t, http.StatusOK, status, "%s", body)
	require.Contains(t, string(body), testAdminUsername)
	assertNoSecrets(t, body)

	knownStatus, knownBody := signIn(testAdminUsername, "incorrect-password")
	unknownStatus, unknownBody := signIn("missing-account", "incorrect-password")
	require.GreaterOrEqual(t, knownStatus, http.StatusBadRequest)
	require.Less(t, knownStatus, http.StatusInternalServerError)
	require.Equal(t, knownStatus, unknownStatus)
	require.JSONEq(t, string(knownBody), string(unknownBody))
	require.NotContains(t, string(knownBody), testAdminUsername)
	assertNoSecrets(t, knownBody)
}

func TestBrowserSessionOrigin(t *testing.T) {
	ctx := context.Background()
	const canonicalOrigin = "https://cat-two.example.test"
	inst := bootInstance(ctx, t, instanceOptions{personal: true, instanceURL: canonicalOrigin})
	inst.createAdmin(t)
	credentials := map[string]any{
		"passwordCredentials": map[string]string{"username": testAdminUsername, "password": testAdminPassword},
	}

	type authResponse struct {
		status  int
		headers http.Header
		cookies []*http.Cookie
	}
	post := func(t *testing.T, path, origin, fetchSite string, body any, cookie *http.Cookie, legacyCookie string) (authResponse, string) {
		t.Helper()
		req := inst.connectRequest(t, path, "", body)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if fetchSite != "" {
			req.Header.Set("Sec-Fetch-Site", fetchSite)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if legacyCookie != "" {
			req.Header.Set("Grpc-Metadata-Cookie", legacyCookie)
		}
		response, err := inst.client.Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		message, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return authResponse{status: response.StatusCode, headers: response.Header, cookies: response.Cookies()}, string(message)
	}
	for _, origin := range []struct{ url, fetchSite string }{
		{"https://untrusted.example.test", "same-site"},
		{"null", "cross-site"},
	} {
		response, body := post(t, apiconnect.AuthServiceSignInProcedure, origin.url, origin.fetchSite, credentials, nil, "")
		require.Equal(t, http.StatusForbidden, response.status, "%s", body)
		require.Empty(t, response.cookies)
		require.Contains(t, response.headers.Get("Cache-Control"), "no-store")
	}
	for _, origin := range []struct{ url, fetchSite string }{
		{canonicalOrigin, ""},
		{inst.baseURL, ""},
		{"https://proxy-rewritten.example.test", "same-origin"},
		{"", ""},
	} {
		response, body := post(t, apiconnect.AuthServiceSignInProcedure, origin.url, origin.fetchSite, credentials, nil, "")
		require.Equal(t, http.StatusOK, response.status, "%s", body)
		require.Len(t, response.cookies, 1)
		cookie := response.cookies[0]
		require.True(t, cookie.HttpOnly)
		require.True(t, cookie.Secure)
		require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
		require.Empty(t, response.headers.Values("Grpc-Metadata-Set-Cookie"))
		response, body = post(t, apiconnect.AuthServiceRefreshTokenProcedure, "https://untrusted.example.test", "same-site", nil, cookie, "")
		require.Equal(t, http.StatusForbidden, response.status, "%s", body)
		require.Empty(t, response.cookies)
		response, body = post(t, apiconnect.AuthServiceRefreshTokenProcedure, origin.url, origin.fetchSite, nil, cookie, "")
		require.Equal(t, http.StatusOK, response.status, "%s", body)
		require.Len(t, response.cookies, 1)
		require.NotEqual(t, cookie.Value, response.cookies[0].Value)
		require.Empty(t, response.headers.Values("Grpc-Metadata-Set-Cookie"))
		require.Contains(t, response.headers.Get("Cache-Control"), "no-store")
	}

	t.Run("legacy gateway cookie header is ignored", func(t *testing.T) {
		response, body := post(t, apiconnect.AuthServiceSignInProcedure, "", "", credentials, nil, "")
		require.Equal(t, http.StatusOK, response.status, "%s", body)
		require.Len(t, response.cookies, 1)
		cookie := response.cookies[0]
		response, body = post(t, apiconnect.AuthServiceRefreshTokenProcedure, "", "", nil, nil, cookie.Name+"="+cookie.Value)
		require.Equal(t, http.StatusUnauthorized, response.status, "%s", body)
		require.Empty(t, response.cookies)
		response, body = post(t, apiconnect.AuthServiceRefreshTokenProcedure, "", "", nil, cookie, cookie.Name+"=invalid-value")
		require.Equal(t, http.StatusOK, response.status, "%s", body)
		require.Len(t, response.cookies, 1)
	})
}
