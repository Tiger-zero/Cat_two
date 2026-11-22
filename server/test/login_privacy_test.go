package test

import (
	"bytes"
	"context"
	"encoding/json"
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
	status, created := inst.do(t, http.MethodPost, "/api/users", "", map[string]string{
		"username": testAdminUsername, "password": testAdminPassword, "email": email,
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

	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/auth"},
		{http.MethodGet, "/auth/admin"},
		{http.MethodGet, "/api/instance/profile"},
		{http.MethodPost, apiconnect.InstanceServiceGetInstanceProfileProcedure},
		{http.MethodGet, "/api/instance/settings/BASIC"},
		{http.MethodGet, "/api/instance/settings/GENERAL"},
	} {
		t.Run(route.path, func(t *testing.T) {
			var request any
			if route.method == http.MethodPost {
				request = map[string]string{}
			}
			status, body := inst.do(t, route.method, route.path, "", request)
			require.Equal(t, http.StatusOK, status, "%s", body)
			require.NotContains(t, string(body), testAdminUsername)
			require.NotContains(t, string(body), email)
			assertNoSecrets(t, body)
		})
	}
	for _, route := range []string{"/api/users/" + testAdminUsername, "/api/auth/me", "/file/users/" + testAdminUsername + "/avatar"} {
		status, body := inst.do(t, http.MethodGet, route, "", nil)
		require.Equal(t, http.StatusUnauthorized, status, "%s: %s", route, body)
		require.NotContains(t, string(body), email)
		assertNoSecrets(t, body)
	}

	for _, path := range []string{"/api/auth/signin", apiconnect.AuthServiceSignInProcedure} {
		t.Run(path, func(t *testing.T) {
			signIn := func(username, password string) (int, []byte) {
				return inst.do(t, http.MethodPost, path, "", map[string]any{
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
		})
	}
}

func TestBrowserSessionOrigin(t *testing.T) {
	ctx := context.Background()
	const canonicalOrigin = "https://cat-two.example.test"
	inst := bootInstance(ctx, t, instanceOptions{personal: true, instanceURL: canonicalOrigin})
	inst.createAdmin(t)
	credentials, err := json.Marshal(map[string]any{
		"passwordCredentials": map[string]string{"username": testAdminUsername, "password": testAdminPassword},
	})
	require.NoError(t, err)

	type authResponse struct {
		status  int
		headers http.Header
		cookies []*http.Cookie
	}
	post := func(t *testing.T, path, origin, fetchSite string, body []byte, cookie *http.Cookie, legacyCookie string) (authResponse, string) {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, inst.baseURL+path, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
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
	for _, transport := range []struct {
		name, signIn, refresh string
	}{
		{"REST", "/api/auth/signin", "/api/auth/refresh"},
		{"Connect", apiconnect.AuthServiceSignInProcedure, apiconnect.AuthServiceRefreshTokenProcedure},
	} {
		t.Run(transport.name, func(t *testing.T) {
			for _, origin := range []struct{ url, fetchSite string }{
				{"https://untrusted.example.test", "same-site"},
				{"null", "cross-site"},
			} {
				response, body := post(t, transport.signIn, origin.url, origin.fetchSite, credentials, nil, "")
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
				response, body := post(t, transport.signIn, origin.url, origin.fetchSite, credentials, nil, "")
				require.Equal(t, http.StatusOK, response.status, "%s", body)
				require.Len(t, response.cookies, 1)
				cookie := response.cookies[0]
				require.True(t, cookie.HttpOnly)
				require.True(t, cookie.Secure)
				require.Equal(t, http.SameSiteLaxMode, cookie.SameSite)
				require.Empty(t, response.headers.Values("Grpc-Metadata-Set-Cookie"))
				response, body = post(t, transport.refresh, "https://untrusted.example.test", "same-site", []byte("{}"), cookie, "")
				require.Equal(t, http.StatusForbidden, response.status, "%s", body)
				require.Empty(t, response.cookies)
				response, body = post(t, transport.refresh, origin.url, origin.fetchSite, []byte("{}"), cookie, "")
				require.Equal(t, http.StatusOK, response.status, "%s", body)
				require.Len(t, response.cookies, 1)
				require.NotEqual(t, cookie.Value, response.cookies[0].Value)
				require.Empty(t, response.headers.Values("Grpc-Metadata-Set-Cookie"))
				require.Contains(t, response.headers.Get("Cache-Control"), "no-store")
			}
		})
	}

	t.Run("REST legacy cookie input", func(t *testing.T) {
		response, body := post(t, "/api/auth/signin", "", "", credentials, nil, "")
		require.Equal(t, http.StatusOK, response.status, "%s", body)
		require.Len(t, response.cookies, 1)
		legacyCookie := response.cookies[0]
		response, body = post(t, "/api/auth/refresh", "", "", []byte("{}"), nil, legacyCookie.Name+"="+legacyCookie.Value)
		require.Equal(t, http.StatusOK, response.status, "%s", body)
		require.Len(t, response.cookies, 1)
		// When both are present, the real HTTP cookie wins over stale legacy metadata.
		response, body = post(t, "/api/auth/refresh", "", "", []byte("{}"), response.cookies[0], legacyCookie.Name+"="+legacyCookie.Value)
		require.Equal(t, http.StatusOK, response.status, "%s", body)
	})
}
