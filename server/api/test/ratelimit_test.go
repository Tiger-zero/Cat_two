package test

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/internal/clientip"
	"github.com/usememos/memos/internal/ratelimit"
	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

// limitedPolicy returns the default table with one scope tightened.
func limitedPolicy(scope ratelimit.Scope, limit int) ratelimit.Policy {
	policy := ratelimit.DefaultPolicy()
	policy[scope] = ratelimit.Rule{Limit: limit, Window: time.Hour}
	return policy
}

func withClientIP(ctx context.Context, ip string) context.Context {
	return clientip.WithClientIP(ctx, ip)
}

func requireRateLimited(t *testing.T, err error, scope ratelimit.Scope) {
	t.Helper()
	st := status.Convert(err)
	require.Equal(t, codes.ResourceExhausted, st.Code(), "got %v", err)
	for _, detail := range st.Details() {
		if info, ok := detail.(*errdetails.ErrorInfo); ok {
			require.Equal(t, "RATE_LIMITED", info.Reason)
			require.Equal(t, string(scope), info.Metadata["scope"])
			retryAfter, err := strconv.Atoi(info.Metadata["retry_after_seconds"])
			require.NoError(t, err)
			require.Positive(t, retryAfter)
			return
		}
	}
	t.Fatalf("no ErrorInfo detail on %v", err)
}

func passwordSignIn(ctx context.Context, ts *TestService, username, password string) error {
	_, err := ts.Service.SignIn(api.WithHeaderCarrier(ctx), &apipb.SignInRequest{
		Credentials: &apipb.SignInRequest_PasswordCredentials_{
			PasswordCredentials: &apipb.SignInRequest_PasswordCredentials{Username: username, Password: password},
		},
	})
	return err
}

func TestSignInRateLimits(t *testing.T) {
	ctx := withClientIP(context.Background(), "203.0.113.10")

	t.Run("failures count and the check precedes the credential check", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		ts.Service.RateLimiter = ratelimit.NewMemoryLimiter(limitedPolicy(ratelimit.ScopeSignInAccount, 2))
		createPasswordUser(ctx, t, ts, "alice", "correct-password")

		require.Equal(t, codes.InvalidArgument, status.Code(passwordSignIn(ctx, ts, "alice", "wrong")))
		require.Equal(t, codes.InvalidArgument, status.Code(passwordSignIn(ctx, ts, "alice", "wrong")))
		// The third attempt is refused even with the right password: the limit
		// is checked before the password is looked at.
		requireRateLimited(t, passwordSignIn(ctx, ts, "alice", "correct-password"), ratelimit.ScopeSignInAccount)
		// Another account is unaffected.
		createPasswordUser(ctx, t, ts, "bob", "bob-password")
		require.NoError(t, passwordSignIn(ctx, ts, "bob", "bob-password"))
	})

	t.Run("successes do not count", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		ts.Service.RateLimiter = ratelimit.NewMemoryLimiter(limitedPolicy(ratelimit.ScopeSignInAccount, 1))
		createPasswordUser(ctx, t, ts, "alice", "correct-password")
		for range 5 {
			require.NoError(t, passwordSignIn(ctx, ts, "alice", "correct-password"))
		}
	})

	t.Run("the account limit does not reveal whether the account exists", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		ts.Service.RateLimiter = ratelimit.NewMemoryLimiter(limitedPolicy(ratelimit.ScopeSignInAccount, 1))
		createPasswordUser(ctx, t, ts, "alice", "correct-password")

		require.Equal(t, codes.InvalidArgument, status.Code(passwordSignIn(ctx, ts, "alice", "wrong")))
		require.Equal(t, codes.InvalidArgument, status.Code(passwordSignIn(ctx, ts, "nobody", "wrong")))
		requireRateLimited(t, passwordSignIn(ctx, ts, "alice", "wrong"), ratelimit.ScopeSignInAccount)
		requireRateLimited(t, passwordSignIn(ctx, ts, "nobody", "wrong"), ratelimit.ScopeSignInAccount)
	})

	t.Run("the address limit is keyed by resolved address", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		ts.Service.RateLimiter = ratelimit.NewMemoryLimiter(limitedPolicy(ratelimit.ScopeSignInIP, 1))

		require.Equal(t, codes.InvalidArgument, status.Code(passwordSignIn(ctx, ts, "nobody", "wrong")))
		requireRateLimited(t, passwordSignIn(ctx, ts, "someone-else", "wrong"), ratelimit.ScopeSignInIP)
		other := withClientIP(context.Background(), "203.0.113.11")
		require.Equal(t, codes.InvalidArgument, status.Code(passwordSignIn(other, ts, "nobody", "wrong")))
	})

	t.Run("concurrent attempts cannot share one remaining unit", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		ts.Service.RateLimiter = ratelimit.NewMemoryLimiter(limitedPolicy(ratelimit.ScopeSignInAccount, 2))
		createPasswordUser(ctx, t, ts, "alice", "correct-password")

		const racers = 16
		results := make([]codes.Code, racers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range racers {
			wg.Go(func() {
				<-start
				results[i] = status.Code(passwordSignIn(ctx, ts, "alice", "wrong"))
			})
		}
		close(start)
		wg.Wait()
		guesses := 0
		for _, code := range results {
			switch code {
			case codes.InvalidArgument:
				guesses++
			case codes.ResourceExhausted:
			default:
				t.Fatalf("unexpected code %v", code)
			}
		}
		require.Equal(t, 2, guesses, "each attempt reserves its unit before the credential check")
	})

	t.Run("a nil limiter leaves sign-in unbounded", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		for range 20 {
			require.Equal(t, codes.InvalidArgument, status.Code(passwordSignIn(ctx, ts, "nobody", "wrong")))
		}
	})
}

func TestCreateUserRateLimits(t *testing.T) {
	ctx := withClientIP(context.Background(), "203.0.113.20")
	signup := func(ts *TestService, ctx context.Context, username string, validateOnly bool) error {
		_, err := ts.Service.CreateUser(ctx, &apipb.CreateUserRequest{
			User:         &apipb.User{Username: username, Password: "password123"},
			ValidateOnly: validateOnly,
		})
		return err
	}

	t.Run("initial setup bounds validation and creation separately", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		policy := limitedPolicy(ratelimit.ScopeSignupIP, 1)
		policy[ratelimit.ScopeValidateIP] = ratelimit.Rule{Limit: 1, Window: time.Hour}
		limiter := ratelimit.NewMemoryLimiter(policy)
		ts.Service.RateLimiter = limiter

		require.NoError(t, signup(ts, ctx, "owner", true))
		requireRateLimited(t, signup(ts, ctx, "owner", true), ratelimit.ScopeValidateIP)
		const otherIP = "203.0.113.21"
		limiter.Hit(ratelimit.ScopeSignupIP, otherIP, 1)
		requireRateLimited(t, signup(ts, withClientIP(t.Context(), otherIP), "owner", false), ratelimit.ScopeSignupIP)
		require.NoError(t, signup(ts, ctx, "owner", false))
		require.Equal(t, codes.PermissionDenied, status.Code(signup(ts, ctx, "other", false)))
	})
}

type fakeChallenge struct {
	accept string
	seen   []string
}

func (*fakeChallenge) Provider() string { return "fake" }
func (*fakeChallenge) SiteKey() string  { return "site-key" }
func (c *fakeChallenge) Verify(_ context.Context, _ ratelimit.Scope, token string, clientIP string) error {
	c.seen = append(c.seen, clientIP)
	if token != c.accept {
		return errors.New("bad token")
	}
	return nil
}

func withChallengeToken(ctx context.Context, token string) context.Context {
	return metadata.NewIncomingContext(ctx, metadata.Pairs("challenge-token", token))
}

func requireChallengeRequired(t *testing.T, err error) {
	t.Helper()
	st := status.Convert(err)
	require.Equal(t, codes.FailedPrecondition, st.Code(), "got %v", err)
	require.Len(t, st.Details(), 1)
	info, ok := st.Details()[0].(*errdetails.ErrorInfo)
	require.True(t, ok)
	require.Equal(t, "CHALLENGE_REQUIRED", info.Reason)
}

func TestChallengeSeam(t *testing.T) {
	ctx := withClientIP(context.Background(), "203.0.113.30")

	t.Run("initial setup skips the sign-in challenge", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		challenge := &fakeChallenge{accept: "good"}
		ts.Service.Challenge = challenge

		_, err := ts.Service.CreateUser(ctx, &apipb.CreateUserRequest{
			User: &apipb.User{Username: "owner", Password: "password123"},
		})
		require.NoError(t, err)
		require.Empty(t, challenge.seen)
		profile, err := ts.Service.GetInstanceProfile(ctx, &apipb.GetInstanceProfileRequest{})
		require.NoError(t, err)
		require.Equal(t, "fake", profile.GetChallenge().GetProvider())
		require.Equal(t, "site-key", profile.GetChallenge().GetSiteKey())
	})

	t.Run("password sign-in requires a valid token", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		createPasswordUser(ctx, t, ts, "alice", "correct-password")
		ts.Service.Challenge = &fakeChallenge{accept: "good"}

		requireChallengeRequired(t, passwordSignIn(ctx, ts, "alice", "correct-password"))
		require.NoError(t, passwordSignIn(withChallengeToken(ctx, "good"), ts, "alice", "correct-password"))
	})

	t.Run("no challenge configured means no profile entry and no requirement", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		profile, err := ts.Service.GetInstanceProfile(ctx, &apipb.GetInstanceProfileRequest{})
		require.NoError(t, err)
		require.Nil(t, profile.GetChallenge())
	})
}

func TestLinkMetadataRateLimit(t *testing.T) {
	ctx := withClientIP(context.Background(), "203.0.113.50")
	ts := NewTestService(t)
	defer ts.Cleanup()
	limiter := ratelimit.NewMemoryLimiter(limitedPolicy(ratelimit.ScopeLinkMetadata, 2))
	ts.Service.RateLimiter = limiter

	// Three URLs exceed the budget of two, so the batch is refused before any
	// fetch: these hosts do not exist and would otherwise fail differently.
	_, err := ts.Service.BatchGetLinkMetadata(ctx, &apipb.BatchGetLinkMetadataRequest{Urls: []string{
		"http://a.invalid/", "http://b.invalid/", "http://c.invalid/",
	}})
	requireRateLimited(t, err, ratelimit.ScopeLinkMetadata)
	require.Equal(t, 2, limiter.Allowed(ratelimit.ScopeLinkMetadata, "203.0.113.50", 1).Remaining, "a refused batch costs nothing")

	// An authenticated caller is keyed by user, not address: spending the
	// user's budget elsewhere refuses them while the address stays untouched.
	user, err := ts.CreateRegularUser(ctx, "reader")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)
	limiter.Hit(ratelimit.ScopeLinkMetadata, strconv.Itoa(int(user.ID)), 2)
	_, err = ts.Service.GetLinkMetadata(userCtx, &apipb.GetLinkMetadataRequest{Url: "http://a.invalid/"})
	requireRateLimited(t, err, ratelimit.ScopeLinkMetadata)
	require.Equal(t, 2, limiter.Allowed(ratelimit.ScopeLinkMetadata, "203.0.113.50", 1).Remaining)
}

func TestWriteAndUploadBudgets(t *testing.T) {
	ctx := withClientIP(context.Background(), "203.0.113.60")
	ts := NewTestService(t)
	defer ts.Cleanup()
	ts.Service.RateLimiter = ratelimit.NewMemoryLimiter(limitedPolicy(ratelimit.ScopeWriteUser, 2))
	user, err := ts.CreateRegularUser(ctx, "writer")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	createMemo := func(content string) error {
		_, err := ts.Service.CreateMemo(userCtx, &apipb.CreateMemoRequest{Memo: &apipb.Memo{Content: content, Visibility: apipb.Visibility_PRIVATE}})
		return err
	}
	require.NoError(t, createMemo("one"))
	require.NoError(t, createMemo("two"))
	requireRateLimited(t, createMemo("three"), ratelimit.ScopeWriteUser)

	// Another user has their own budget.
	other, err := ts.CreateRegularUser(ctx, "other")
	require.NoError(t, err)
	_, err = ts.Service.CreateMemo(ts.CreateUserContext(ctx, other.ID), &apipb.CreateMemoRequest{Memo: &apipb.Memo{Content: "hi", Visibility: apipb.Visibility_PRIVATE}})
	require.NoError(t, err)
}

func TestAuthorizerBudgets(t *testing.T) {
	ctx := withClientIP(context.Background(), "203.0.113.70")
	ts := NewTestService(t)
	defer ts.Cleanup()
	policy := ratelimit.DefaultPolicy()
	policy[ratelimit.ScopeAnonymous] = ratelimit.Rule{Limit: 2, Window: time.Hour}
	policy[ratelimit.ScopeAuthenticated] = ratelimit.Rule{Limit: 3, Window: time.Hour}
	limiter := ratelimit.NewMemoryLimiter(policy)
	authorizer := api.NewAuthorizer(ts.Store, ts.Secret).WithRateLimiter(limiter)
	const procedure = "/memos.api.MemoService/ListMemos"

	// Anonymous callers share a budget per address.
	require.NoError(t, authorizer.Throttle(ctx, procedure, nil))
	require.NoError(t, authorizer.Throttle(ctx, procedure, nil))
	requireRateLimited(t, authorizer.Throttle(ctx, procedure, nil), ratelimit.ScopeAnonymous)
	require.NoError(t, authorizer.Throttle(withClientIP(context.Background(), "203.0.113.71"), procedure, nil))

	// Different web sessions of the same user share a request budget.
	claims := &auth.AuthResult{Claims: &auth.UserClaims{UserID: 7}}
	otherSession := &auth.AuthResult{Claims: &auth.UserClaims{UserID: 7, SessionID: "other-session"}}
	require.NoError(t, authorizer.Throttle(ctx, procedure, claims))
	require.NoError(t, authorizer.Throttle(ctx, procedure, otherSession))
	require.NoError(t, authorizer.Throttle(ctx, procedure, claims))
	requireRateLimited(t, authorizer.Throttle(ctx, procedure, otherSession), ratelimit.ScopeAuthenticated)
	require.NoError(t, authorizer.Throttle(ctx, procedure, &auth.AuthResult{Claims: &auth.UserClaims{UserID: 8}}))

	// A request with no resolvable address is not counted rather than lumped together.
	require.NoError(t, authorizer.Throttle(context.Background(), procedure, nil))
	require.NoError(t, authorizer.Throttle(context.Background(), procedure, nil))
	require.NoError(t, authorizer.Throttle(context.Background(), procedure, nil))

	// Without a limiter nothing is bounded.
	unlimited := api.NewAuthorizer(ts.Store, ts.Secret)
	for range 10 {
		require.NoError(t, unlimited.Throttle(ctx, procedure, nil))
	}
}

func TestBatchRequestsCostTheirItemCount(t *testing.T) {
	ctx := withClientIP(context.Background(), "203.0.113.80")
	ts := NewTestService(t)
	defer ts.Cleanup()
	limiter := ratelimit.NewMemoryLimiter(ratelimit.DefaultPolicy())
	ts.Service.RateLimiter = limiter
	user, err := ts.CreateRegularUser(ctx, "batcher")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	before := limiter.Allowed(ratelimit.ScopeAuthenticated, strconv.Itoa(int(user.ID)), 1).Remaining
	_, err = ts.Service.BatchGetUsers(userCtx, &apipb.BatchGetUsersRequest{Names: []string{"users/a", "users/b", "users/c", "users/d"}})
	require.NoError(t, err)
	after := limiter.Allowed(ratelimit.ScopeAuthenticated, strconv.Itoa(int(user.ID)), 1).Remaining
	require.Equal(t, 3, before-after, "the authorizer counts one; the handler adds the other three")
}

// createPasswordUser creates a user who can sign in with the given password.
func createPasswordUser(ctx context.Context, t *testing.T, ts *TestService, username, password string) *store.User {
	t.Helper()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	require.NoError(t, err)
	user, err := ts.Store.CreateUser(ctx, &store.User{
		Username:     username,
		Role:         store.RoleUser,
		Email:        testEmailForUsername(username),
		PasswordHash: string(passwordHash),
	})
	require.NoError(t, err)
	return user
}
