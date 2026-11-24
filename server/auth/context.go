package auth

import (
	"context"
	"time"

	"github.com/usememos/memos/store"
)

// ContextKey is the key type for context values.
// Using a custom type prevents collisions with other packages.
type ContextKey int

const (
	// UserIDContextKey stores the authenticated user's ID.
	// Set for all authenticated requests.
	// Use GetUserID(ctx) to retrieve this value.
	UserIDContextKey ContextKey = iota

	// UserClaimsContextKey stores the claims from access token.
	UserClaimsContextKey

	// RefreshTokenIDContextKey stores the refresh token ID.
	RefreshTokenIDContextKey
)

// GetUserID retrieves the authenticated user's ID from the context.
// Returns 0 if no user ID is set (unauthenticated request).
func GetUserID(ctx context.Context) int32 {
	if v, ok := ctx.Value(UserIDContextKey).(int32); ok {
		return v
	}
	return 0
}

// SetUserInContext attaches an identity already verified by its transport.
func SetUserInContext(ctx context.Context, user *store.User) context.Context {
	return context.WithValue(ctx, UserIDContextKey, user.ID)
}

// UserClaims represents authenticated user info from access token.
type UserClaims struct {
	UserID          int32
	Username        string
	Role            string
	Status          string
	SessionRevision string
	SessionID       string
	IdleExpiresAt   time.Time
}

// GetUserClaims retrieves the user claims from context.
// Returns nil if not authenticated via access token.
func GetUserClaims(ctx context.Context) *UserClaims {
	if v, ok := ctx.Value(UserClaimsContextKey).(*UserClaims); ok {
		return v
	}
	return nil
}

// SetUserClaimsInContext sets the user claims in context.
func SetUserClaimsInContext(ctx context.Context, claims *UserClaims) context.Context {
	return context.WithValue(ctx, UserClaimsContextKey, claims)
}

// ApplyToContext sets the authenticated identity from an AuthResult into the context.
// This is the canonical way to propagate auth state after a successful Authenticate call.
// Safe to call with a nil result (no-op).
func ApplyToContext(ctx context.Context, result *AuthResult) context.Context {
	if result == nil || result.Claims == nil {
		return ctx
	}
	ctx = SetUserClaimsInContext(ctx, result.Claims)
	return context.WithValue(ctx, UserIDContextKey, result.Claims.UserID)
}
