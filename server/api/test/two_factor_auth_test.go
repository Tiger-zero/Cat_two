package test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

func TestTwoFactorAuthenticationAndSessionReset(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	t.Cleanup(ts.Cleanup)
	handler, err := ts.Service.NewHandler(ctx)
	require.NoError(t, err)
	const password = "integration password"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	user, err := ts.Store.CreateUser(ctx, &store.User{Username: "authenticator-owner", Role: store.RoleAdmin, PasswordHash: string(hash)})
	require.NoError(t, err)
	pat, err := ts.Service.CreatePersonalAccessToken(ts.CreateUserContext(ctx, user.ID), &apipb.CreatePersonalAccessTokenRequest{
		Parent: "users/" + user.Username, Description: "integration client",
	})
	require.NoError(t, err)

	call := func(method, path, token string, cookie *http.Cookie, payload any) *httptest.ResponseRecorder {
		t.Helper()
		var encoded []byte
		if payload != nil {
			var err error
			encoded, err = json.Marshal(payload)
			require.NoError(t, err)
		}
		req := httptest.NewRequestWithContext(ctx, method, path, bytes.NewReader(encoded))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}

	signIn := func(otp string) *httptest.ResponseRecorder {
		t.Helper()
		return call(http.MethodPost, "/memos.api.AuthService/SignIn", "", nil, map[string]any{
			"passwordCredentials": map[string]string{"username": user.Username, "password": password, "otpCode": otp},
		})
	}
	refresh := func(cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		return call(http.MethodPost, "/memos.api.AuthService/RefreshToken", "", cookie, map[string]any{})
	}
	session := func(response *httptest.ResponseRecorder) (string, *http.Cookie) {
		t.Helper()
		require.Equal(t, http.StatusOK, response.Code, "%s", response.Body)
		var credentials struct {
			AccessToken string `json:"accessToken"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &credentials))
		require.NotEmpty(t, credentials.AccessToken)
		result := response.Result()
		defer result.Body.Close()
		cookies := result.Cookies()
		require.Len(t, cookies, 1)
		require.Equal(t, auth.RefreshTokenCookieName, cookies[0].Name)
		return credentials.AccessToken, cookies[0]
	}
	requireStatus := func(response *httptest.ResponseRecorder, want int) {
		t.Helper()
		require.Equal(t, want, response.Code, "%s", response.Body)
	}
	rejectSignIn := func(response *httptest.ResponseRecorder, want int, reason string) {
		t.Helper()
		requireStatus(response, want)
		require.Empty(t, response.Header().Get("Set-Cookie"))
		require.NotContains(t, response.Body.String(), "accessToken")
		require.Contains(t, response.Body.String(), reason)
	}

	preEnrollmentAccess, preEnrollmentRefresh := session(signIn(""))
	requireStatus(call(http.MethodGet, "/api/auth/totp", pat.GetToken(), nil, nil), http.StatusForbidden)
	requireStatus(call(http.MethodGet, "/api/auth/totp", "", nil, nil), http.StatusUnauthorized)
	requireStatus(call(http.MethodPost, "/api/auth/totp/setup", preEnrollmentAccess, nil, map[string]string{"password": "wrong"}), http.StatusBadRequest)

	setup := call(http.MethodPost, "/api/auth/totp/setup", preEnrollmentAccess, nil, map[string]string{"password": password})
	requireStatus(setup, http.StatusOK)
	require.Contains(t, setup.Header().Get("Cache-Control"), "no-store")
	var provision struct {
		Secret     string `json:"secret"`
		OTPAuthURL string `json:"otpauthUrl"`
	}
	require.NoError(t, json.Unmarshal(setup.Body.Bytes(), &provision))
	setupURI, err := url.Parse(provision.OTPAuthURL)
	require.NoError(t, err)
	require.Equal(t, "otpauth", setupURI.Scheme)
	require.Equal(t, provision.Secret, setupURI.Query().Get("secret"))
	require.NotEmpty(t, provision.Secret)
	pending, err := ts.Store.GetUserTwoFactor(ctx, user.ID)
	require.NoError(t, err)
	require.NotEmpty(t, pending.PendingSecret)
	require.NotContains(t, pending.PendingSecret, provision.Secret)
	require.Empty(t, pending.ActiveSecret)

	enrollCode := generateIntegrationOTP(t, provision.Secret, time.Now().Unix()/30)
	requireStatus(call(http.MethodPost, "/api/auth/totp/enable", preEnrollmentAccess, nil, map[string]string{
		"password": password, "code": enrollCode,
	}), http.StatusOK)
	requireStatus(call(http.MethodGet, "/api/auth/me", preEnrollmentAccess, nil, nil), http.StatusUnauthorized)
	requireStatus(refresh(preEnrollmentRefresh), http.StatusUnauthorized)
	requireStatus(call(http.MethodGet, "/api/auth/me", pat.GetToken(), nil, nil), http.StatusOK)

	rejectSignIn(signIn(""), http.StatusBadRequest, "TOTP_REQUIRED")
	rejectSignIn(call(http.MethodPost, "/api/auth/signin", "", nil, map[string]any{
		"passwordCredentials": map[string]string{"username": user.Username, "password": password},
	}), http.StatusBadRequest, "TOTP_REQUIRED")
	rejectSignIn(signIn(enrollCode), http.StatusBadRequest, "TOTP_INVALID")

	// An adjacent fresh code makes this test independent of the wall-clock
	// boundary without weakening the real enrollment/replay lifecycle.
	loginCode := generateIntegrationOTP(t, provision.Secret, time.Now().Unix()/30+1)
	var results [2]*httptest.ResponseRecorder
	var requests sync.WaitGroup
	for i := range results {
		requests.Go(func() { results[i] = signIn(loginCode) })
	}
	requests.Wait()
	accepted, replayed := results[0], results[1]
	if accepted.Code != http.StatusOK {
		accepted, replayed = replayed, accepted
	}
	enrolledAccess, enrolledRefresh := session(accepted)
	rejectSignIn(replayed, http.StatusBadRequest, "TOTP_INVALID")
	requireStatus(call(http.MethodGet, "/api/auth/totp", enrolledAccess, nil, nil), http.StatusOK)
	refreshedAccess, refreshedRefresh := session(refresh(enrolledRefresh))

	// The persisted budget does not depend on the optional HTTP rate limiter.
	for range 4 {
		rejectSignIn(signIn("invalid"), http.StatusBadRequest, "TOTP_INVALID")
	}
	rejectSignIn(signIn("invalid"), http.StatusTooManyRequests, "RATE_LIMITED")

	require.NoError(t, ts.Store.ResetUserTwoFactor(ctx, user.ID))
	reset, err := ts.Store.GetUserTwoFactor(ctx, user.ID)
	require.NoError(t, err)
	require.Empty(t, reset.ActiveSecret)
	require.Empty(t, reset.PendingSecret)
	requireStatus(call(http.MethodGet, "/api/auth/me", refreshedAccess, nil, nil), http.StatusUnauthorized)
	requireStatus(refresh(refreshedRefresh), http.StatusUnauthorized)
	requireStatus(call(http.MethodGet, "/api/auth/me", pat.GetToken(), nil, nil), http.StatusOK)
	access, _ := session(signIn(""))
	requireStatus(call(http.MethodGet, "/api/auth/me", access, nil, nil), http.StatusOK)
}

func generateIntegrationOTP(t *testing.T, secret string, step int64) string {
	t.Helper()
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	require.NoError(t, err)
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 0xf
	code := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1_000_000)
}
