package auth

import (
	"context"
	"crypto/hmac"
	"log/slog"
	"strconv"
	"time"

	"github.com/pkg/errors"

	"github.com/usememos/memos/store"
)

// Authenticator provides shared authentication and authorization logic.
// Used by gRPC interceptor, Connect interceptor, and file server to ensure
// consistent authentication behavior across all API endpoints.
// It authenticates short-lived JWTs and refresh cookies for web sessions.
type Authenticator struct {
	store  *store.Store
	secret string
}

// ErrSessionIdleExpired means a verified web session outlasted its edit deadline.
var ErrSessionIdleExpired = errors.New("session idle expired")

// ErrInvalidRefreshToken denotes credentials that cannot authenticate. Store
// errors remain separate so callers can distinguish a backend failure.
var ErrInvalidRefreshToken = errors.New("invalid refresh token")

// NewAuthenticator creates a new Authenticator instance.
func NewAuthenticator(store *store.Store, secret string) *Authenticator {
	return &Authenticator{
		store:  store,
		secret: secret,
	}
}

// AuthenticateByAccessTokenV2 validates a short-lived access token.
// Checks the user's current account state and password-bound session hash.
func (a *Authenticator) AuthenticateByAccessTokenV2(ctx context.Context, accessToken string) (*UserClaims, error) {
	result, err := a.resolveAccessToken(ctx, accessToken)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("invalid access token")
	}
	return result.claims, nil
}

// resolveAccessToken returns nil for invalid credentials and an error for store failures.
func (a *Authenticator) resolveAccessToken(ctx context.Context, accessToken string) (*bearerAuth, error) {
	claims, err := ParseAccessTokenV2(accessToken, []byte(a.secret))
	if err != nil {
		return nil, nil //nolint:nilerr // Invalid credentials permit fallback; only store failures are errors.
	}
	if claims.SessionID == "" && a.store.IsPersonalMode() {
		// Upgraded browsers exchange their existing refresh cookie for a bound token.
		return nil, nil
	}

	userID, err := parseInt32(claims.Subject)
	if err != nil {
		return nil, nil //nolint:nilerr // Invalid credentials permit fallback; only store failures are errors.
	}
	user, err := a.store.GetUser(ctx, &store.FindUser{ID: &userID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get access token user")
	}
	if user == nil || user.RowStatus == store.Archived {
		return nil, nil
	}
	if !hmac.Equal([]byte(claims.AuthHash), []byte(accessTokenAuthHash(user.ID, user.PasswordHash, []byte(a.secret)))) {
		return nil, nil
	}
	factor, err := a.store.GetUserTwoFactor(ctx, userID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get access token session revision")
	}
	if claims.SessionRevision != factor.SessionRevision {
		return nil, nil
	}
	var idleExpiresAt time.Time
	if claims.SessionID != "" {
		active, deadline, err := a.store.IsUserSessionActive(ctx, userID, claims.SessionID)
		if err != nil {
			return nil, errors.Wrap(err, "failed to check access token session")
		}
		if !active {
			if !deadline.IsZero() {
				return nil, ErrSessionIdleExpired
			}
			return nil, nil
		}
		if a.store.IsPersonalMode() {
			idleExpiresAt = deadline
		}
	}

	return &bearerAuth{
		user: user,
		claims: &UserClaims{
			UserID:          userID,
			Username:        claims.Username,
			Role:            claims.Role,
			Status:          claims.Status,
			SessionRevision: claims.SessionRevision,
			SessionID:       claims.SessionID,
			IdleExpiresAt:   idleExpiresAt,
		},
	}, nil
}

// AuthenticateByRefreshToken validates a refresh token against the database.
func (a *Authenticator) AuthenticateByRefreshToken(ctx context.Context, refreshToken string) (*store.User, string, error) {
	user, claims, err := a.AuthenticateRefreshSession(ctx, refreshToken)
	if err != nil {
		return nil, "", err
	}
	return user, claims.TokenID, nil
}

// AuthenticateRefreshSession returns the proven revision alongside the refresh token's user.
func (a *Authenticator) AuthenticateRefreshSession(ctx context.Context, refreshToken string) (*store.User, *RefreshTokenClaims, error) {
	claims, err := ParseRefreshToken(refreshToken, []byte(a.secret))
	if err != nil {
		return nil, nil, ErrInvalidRefreshToken
	}

	userID, err := parseInt32(claims.Subject)
	if err != nil {
		return nil, nil, errors.Wrap(ErrInvalidRefreshToken, "invalid user ID in token")
	}

	// Check token exists in database (revocation check)
	token, err := a.store.GetUserRefreshTokenByID(ctx, userID, claims.TokenID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get refresh token")
	}
	if token == nil || token.GetSessionId() != claims.SessionID {
		return nil, nil, errors.Wrap(ErrInvalidRefreshToken, "refresh token revoked")
	}

	// Check token not expired
	now := time.Now()
	if token.ExpiresAt != nil && !token.ExpiresAt.AsTime().After(now) {
		return nil, nil, errors.Wrap(ErrInvalidRefreshToken, "refresh token expired")
	}
	if a.store.IsPersonalMode() && !store.UserSessionIdleExpiresAt(token).After(now) {
		return nil, nil, ErrSessionIdleExpired
	}

	// Get user
	user, err := a.store.GetUser(ctx, &store.FindUser{ID: &userID})
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get user")
	}
	if user == nil {
		return nil, nil, errors.Wrap(ErrInvalidRefreshToken, "user not found")
	}
	if user.RowStatus == store.Archived {
		return nil, nil, errors.Wrap(ErrInvalidRefreshToken, "user is archived")
	}

	factor, err := a.store.GetUserTwoFactor(ctx, userID)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get refresh token session revision")
	}
	if claims.SessionRevision != factor.SessionRevision {
		return nil, nil, errors.Wrap(ErrInvalidRefreshToken, "refresh token revoked")
	}
	return user, claims, nil
}

// AuthResult contains the authenticated web session's claims and access token.
type AuthResult struct {
	Claims      *UserClaims
	AccessToken string
}

type bearerAuth struct {
	user   *store.User
	claims *UserClaims
}

// VerifyBearer verifies a web access token before reading a request body.
func (a *Authenticator) VerifyBearer(ctx context.Context, authHeader string) bool {
	claims, err := a.AuthenticateByAccessTokenV2(ctx, ExtractBearerToken(authHeader))
	return err == nil && claims != nil
}

// AuthenticateToUser resolves the current request to a *store.User, checking the
// Authorization header first, then falling back to the
// refresh token cookie. Returns (nil, nil) when no valid credentials are present.
func (a *Authenticator) AuthenticateToUser(ctx context.Context, authHeader, cookieHeader string) (*store.User, error) {
	bearer, err := a.resolveAccessToken(ctx, ExtractBearerToken(authHeader))
	if err != nil && !errors.Is(err, ErrSessionIdleExpired) {
		return nil, err
	}
	if bearer != nil {
		return bearer.user, nil
	}

	// Fallback: refresh token cookie.
	if cookieHeader != "" {
		if refreshToken := ExtractRefreshTokenFromCookie(cookieHeader); refreshToken != "" {
			user, _, err := a.AuthenticateByRefreshToken(ctx, refreshToken)
			if errors.Is(err, ErrSessionIdleExpired) || errors.Is(err, ErrInvalidRefreshToken) {
				return nil, nil
			}
			return user, err
		}
	}

	return nil, nil
}

// Authenticate resolves a web access token into an AuthResult,
// returning nil when no valid credentials are present. Unlike AuthenticateToUser it
// ignores the refresh cookie. A store failure also yields nil, so the caller is
// treated as anonymous, but it is logged so operators can tell it apart from a
// rejected token.
func (a *Authenticator) Authenticate(ctx context.Context, authHeader string) *AuthResult {
	token := ExtractBearerToken(authHeader)
	bearer, err := a.resolveAccessToken(ctx, token)
	if errors.Is(err, ErrSessionIdleExpired) {
		return nil
	}
	if err != nil {
		slog.Error("failed to resolve bearer token", "error", err)
		return nil
	}
	if bearer == nil {
		return nil
	}
	return &AuthResult{Claims: bearer.claims, AccessToken: token}
}

func parseInt32(src string) (int32, error) {
	parsed, err := strconv.ParseInt(src, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(parsed), nil
}
