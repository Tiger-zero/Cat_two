package test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

func TestSignOutRevokesRotatedSessionOnly(t *testing.T) {
	ts := NewTestService(t)
	defer ts.Cleanup()
	ctx := t.Context()
	user, err := ts.CreateRegularUser(ctx, "sign-out-owner")
	require.NoError(t, err)
	authenticator := auth.NewAuthenticator(ts.Store, ts.Secret)

	upgrade := func(id string) (*apipb.RefreshTokenResponse, string, string) {
		t.Helper()
		addSession(ctx, t, ts, user.ID, id)
		legacyToken, _, err := auth.GenerateRefreshToken(user.ID, id, []byte(ts.Secret))
		require.NoError(t, err)
		requestCtx := api.WithHeaderCarrier(metadata.NewIncomingContext(ctx, metadata.Pairs("cookie", auth.RefreshTokenCookieName+"="+legacyToken)))
		response, err := ts.Service.RefreshToken(requestCtx, &apipb.RefreshTokenRequest{})
		require.NoError(t, err)
		cookie, err := http.ParseSetCookie(api.GetHeaderCarrier(requestCtx).Get("Set-Cookie"))
		require.NoError(t, err)
		return response, legacyToken, cookie.Value
	}

	access, originalCookie, rotatedCookie := upgrade("rotating-browser")
	otherAccess, _, otherCookie := upgrade("other-browser")
	claims := authenticator.Authenticate(ctx, "Bearer "+access.GetAccessToken()).Claims
	require.Equal(t, "rotating-browser", claims.SessionID)

	ctxWithClaims := auth.SetUserClaimsInContext(ctx, claims)
	// The cookie can be from before the refresh response arrived, or belong to a
	// different session after a new browser sign-in.
	otherRefresh := api.WithHeaderCarrier(metadata.NewIncomingContext(ctx, metadata.Pairs(
		"cookie", auth.RefreshTokenCookieName+"="+otherCookie,
		"x-memos-session-id", claims.SessionID,
	)))
	_, err = ts.Service.RefreshToken(otherRefresh, &apipb.RefreshTokenRequest{})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	require.Empty(t, api.GetHeaderCarrier(otherRefresh).Get("Set-Cookie"))

	logout := api.WithHeaderCarrier(metadata.NewIncomingContext(ctxWithClaims, metadata.Pairs("cookie", auth.RefreshTokenCookieName+"="+otherCookie)))
	_, err = ts.Service.SignOut(logout, &apipb.SignOutRequest{})
	require.NoError(t, err)
	require.Empty(t, api.GetHeaderCarrier(logout).Get("Set-Cookie"), "leave the other session's cookie alone")

	require.Nil(t, authenticator.Authenticate(ctx, "Bearer "+access.GetAccessToken()))
	for _, cookie := range []string{originalCookie, rotatedCookie} {
		_, _, err := authenticator.AuthenticateByRefreshToken(ctx, cookie)
		require.ErrorContains(t, err, "revoked")
	}
	require.NotNil(t, authenticator.Authenticate(ctx, "Bearer "+otherAccess.GetAccessToken()))
	_, _, err = authenticator.AuthenticateByRefreshToken(ctx, otherCookie)
	require.NoError(t, err)
}

func TestRefreshSessionRotatesOnlyOnce(t *testing.T) {
	ts := NewTestService(t)
	defer ts.Cleanup()
	ctx := t.Context()
	user, err := ts.CreateRegularUser(ctx, "rotation-owner")
	require.NoError(t, err)
	addSession(ctx, t, ts, user.ID, "rotation-session")
	addSession(ctx, t, ts, user.ID, "other-session")
	oldToken, _, err := auth.GenerateRefreshToken(user.ID, "rotation-session", []byte(ts.Secret))
	require.NoError(t, err)

	type refreshResult struct {
		response *apipb.RefreshTokenResponse
		cookie   string
		err      error
	}
	const parallelRequests = 8
	start := make(chan struct{})
	results := make(chan refreshResult, parallelRequests)
	refreshCtx := metadata.NewIncomingContext(ctx, metadata.Pairs("cookie", auth.RefreshTokenCookieName+"="+oldToken))
	for range parallelRequests {
		go func() {
			<-start
			requestCtx := api.WithHeaderCarrier(refreshCtx)
			response, err := ts.Service.RefreshToken(requestCtx, &apipb.RefreshTokenRequest{})
			results <- refreshResult{response: response, cookie: api.GetHeaderCarrier(requestCtx).Get("Set-Cookie"), err: err}
		}()
	}
	close(start)
	var accepted []refreshResult
	for range parallelRequests {
		result := <-results
		if result.err == nil {
			accepted = append(accepted, result)
			continue
		}
		require.Equal(t, codes.Unauthenticated, status.Code(result.err))
		require.Nil(t, result.response)
		require.Empty(t, result.cookie)
	}
	require.Len(t, accepted, 1)
	require.NotEmpty(t, accepted[0].response.GetAccessToken())
	cookie, err := http.ParseSetCookie(accepted[0].cookie)
	require.NoError(t, err)
	authenticator := auth.NewAuthenticator(ts.Store, ts.Secret)
	_, replacementID, err := authenticator.AuthenticateByRefreshToken(ctx, cookie.Value)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"other-session", replacementID}, sessionIDs(ctx, t, ts, user.ID))
}

func TestSignOutReportsFailedSessionRevocation(t *testing.T) {
	t.Setenv("DRIVER", "sqlite")
	ts := NewTestService(t)
	defer ts.Cleanup()
	ctx := t.Context()
	user, err := ts.CreateRegularUser(ctx, "sign-out-persistence")
	require.NoError(t, err)
	const sessionID, credentialID = "retained-session", "current-credential"
	require.NoError(t, ts.Store.AddUserRefreshToken(ctx, user.ID, &storepb.RefreshTokensUserSetting_RefreshToken{
		TokenId: credentialID, SessionId: sessionID,
	}))
	access, _, err := auth.GenerateSessionAccessToken(user, sessionID, "", []byte(ts.Secret))
	require.NoError(t, err)
	refresh, _, err := auth.GenerateSessionRefreshToken(user.ID, credentialID, sessionID, "", []byte(ts.Secret))
	require.NoError(t, err)
	_, err = ts.Store.GetDriver().GetDB().ExecContext(ctx, `
		CREATE TRIGGER reject_session_revocation BEFORE UPDATE ON user_setting
		WHEN OLD.key = 'REFRESH_TOKENS'
		BEGIN SELECT RAISE(ABORT, 'test session revocation failure'); END`)
	require.NoError(t, err)
	authenticator := auth.NewAuthenticator(ts.Store, ts.Secret)
	caller := authenticator.Authenticate(ctx, "Bearer "+access)
	require.NotNil(t, caller)
	logoutCtx := api.WithHeaderCarrier(metadata.NewIncomingContext(auth.ApplyToContext(ctx, caller), metadata.Pairs(
		"cookie", auth.RefreshTokenCookieName+"="+refresh,
	)))
	_, err = ts.Service.SignOut(logoutCtx, &apipb.SignOutRequest{})
	require.Equal(t, codes.Internal, status.Code(err))
	require.Empty(t, api.GetHeaderCarrier(logoutCtx).Get("Set-Cookie"))
	require.NotNil(t, authenticator.Authenticate(ctx, "Bearer "+access))
}

func TestSignInDoesNotIssueTokensWhenSessionWriteFails(t *testing.T) {
	t.Setenv("DRIVER", "sqlite")
	ts := NewTestService(t)
	defer ts.Cleanup()
	ctx := api.WithHeaderCarrier(t.Context())
	const password = "test-session-password"
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	user, err := ts.Store.CreateUser(ctx, &store.User{
		Username: "session-owner", Role: store.RoleAdmin, PasswordHash: string(passwordHash),
	})
	require.NoError(t, err)
	_, err = ts.Store.GetDriver().GetDB().ExecContext(ctx, `
		CREATE TRIGGER reject_session_write BEFORE INSERT ON user_setting
		WHEN NEW.key = 'REFRESH_TOKENS'
		BEGIN SELECT RAISE(ABORT, 'test session storage failure'); END`)
	require.NoError(t, err)
	response, err := ts.Service.SignIn(ctx, &apipb.SignInRequest{
		Credentials: &apipb.SignInRequest_PasswordCredentials_{
			PasswordCredentials: &apipb.SignInRequest_PasswordCredentials{Username: user.Username, Password: password},
		},
	})
	require.Equal(t, codes.Internal, status.Code(err))
	require.Nil(t, response)
	require.Empty(t, api.GetHeaderCarrier(ctx).Get("Set-Cookie"))
}
