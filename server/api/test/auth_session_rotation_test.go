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
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

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
