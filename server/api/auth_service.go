package api

import (
	"context"
	"log/slog"

	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/usememos/memos/internal/clientip"
	"github.com/usememos/memos/internal/ratelimit"
	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/store"
)

const (
	unmatchedUsernameAndPasswordError = "unmatched username and password"
	// The same bcrypt cost is used for nonexistent users and accounts with no
	// password, to avoid an immediate error disclosing account existence.
	missingLoginPasswordHash = "$2a$10$OqBAbicTtlqj7Ry4b2CdXuCS4h/XjER3XjisYnM6wHzvo2OCChRji"
)

// GetCurrentUser returns the authenticated user's information.
// Validates the access token and returns user details.
//
// Authentication: Required (access token).
// Returns: User information.
func (s *APIService) GetCurrentUser(ctx context.Context, _ *apipb.GetCurrentUserRequest) (*apipb.GetCurrentUserResponse, error) {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "failed to get current user: %v", err)
	}
	if user == nil {
		// Clear auth cookies
		if err := s.clearAuthCookies(ctx); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to clear auth cookies: %v", err)
		}
		return nil, status.Errorf(codes.Unauthenticated, "user not found")
	}

	return &apipb.GetCurrentUserResponse{
		User: convertUserFromStore(user, user),
	}, nil
}

// SignIn authenticates a user with credentials and returns tokens.
// On success, returns an access token and sets a refresh token cookie.
//
// Supports two authentication methods:
// 1. Password-based authentication (username + password).
// 2. SSO authentication (OAuth2 authorization code).
//
// Authentication: Not required (public endpoint).
// Returns: User info, access token, and token expiry.
func (s *APIService) SignIn(ctx context.Context, request *apipb.SignInRequest) (*apipb.SignInResponse, error) {
	var existingUser *store.User

	// The blocklist and rate limits run before any user lookup or hashing.
	// Each attempt reserves its units up front, so concurrent attempts cannot
	// share one remaining unit; a successful attempt gives them back below.
	// The account key is the submitted name whether or not it exists, so the
	// limit cannot be used to learn which accounts are real.
	clientIP := clientip.FromContext(ctx)
	loginIP := ""
	if s.personalMode() {
		loginIP = canonicalLoginIP(clientIP)
	}
	if loginIP != "" {
		blocked, err := s.Store.IsLoginIPBlocked(ctx, loginIP)
		if err != nil {
			return nil, status.Error(codes.Unavailable, "login protection is currently unavailable")
		}
		if blocked {
			return nil, blockedLoginIPError()
		}
	}
	attempt, err := s.reserveSignIn(clientIP, request.GetPasswordCredentials().GetUsername())
	if err != nil {
		return nil, err
	}

	// Authentication Method 1: Password-based authentication
	if passwordCredentials := request.GetPasswordCredentials(); passwordCredentials != nil {
		if err := s.requireChallenge(ctx, ratelimit.ScopeSignInIP); err != nil {
			attempt.succeeded()
			return nil, err
		}
		user, err := s.Store.GetUser(ctx, &store.FindUser{
			Username: &passwordCredentials.Username,
		})
		if err != nil {
			attempt.succeeded()
			return nil, status.Errorf(codes.Internal, "failed to get user, error: %v", err)
		}
		hash := missingLoginPasswordHash
		if user != nil && user.PasswordHash != "" {
			hash = user.PasswordHash
		}
		match := bcrypt.CompareHashAndPassword([]byte(hash), []byte(passwordCredentials.Password)) == nil
		if user == nil || user.PasswordHash == "" || !match {
			if loginIP != "" {
				_, blocked, err := s.Store.RecordLoginIPFailure(ctx, loginIP, loginIPFailureLimit)
				if err != nil {
					attempt.succeeded()
					return nil, status.Error(codes.Unavailable, "login protection is currently unavailable")
				}
				if blocked {
					slog.InfoContext(ctx, "login IP blocked after failed passwords", "ip", loginIP)
					return nil, blockedLoginIPError()
				}
			}
			return nil, status.Error(codes.InvalidArgument, unmatchedUsernameAndPasswordError)
		}
		instanceGeneralSetting, err := s.Store.GetInstanceGeneralSetting(ctx)
		if err != nil {
			attempt.succeeded()
			return nil, status.Errorf(codes.Internal, "failed to get instance general setting, error: %v", err)
		}
		// Check if the password auth in is allowed.
		if instanceGeneralSetting.DisallowPasswordAuth && user.Role == store.RoleUser {
			attempt.succeeded()
			return nil, status.Errorf(codes.PermissionDenied, "password signin is not allowed")
		}
		existingUser = user
	} else if ssoCredentials := request.GetSsoCredentials(); ssoCredentials != nil {
		// Authentication Method 2: SSO (OAuth2) authentication
		identityProvider, userInfo, err := s.resolveSSOIdentity(ctx, ssoCredentials.IdentityProvider, ssoCredentials.Code, ssoCredentials.RedirectUri, ssoCredentials.CodeVerifier)
		if err != nil {
			// A rejected exchange is a credential failure and stays counted.
			return nil, err
		}
		user, err := s.resolveSSOUser(ctx, nil, identityProvider, userInfo)
		if err != nil {
			// The credential was valid; whatever refused provisioning is not a guess.
			attempt.succeeded()
			return nil, err
		}
		existingUser = user
	}

	if existingUser == nil {
		// No credential was presented at all; nothing to guess against.
		attempt.succeeded()
		return nil, status.Errorf(codes.InvalidArgument, "invalid credentials")
	}
	if existingUser.RowStatus == store.Archived {
		attempt.succeeded()
		return nil, status.Error(codes.PermissionDenied, "user has been archived")
	}

	revision, err := s.verifySignInTwoFactor(ctx, existingUser, request.GetPasswordCredentials().GetOtpCode())
	if err != nil {
		// Missing codes are a normal first step in an interactive sign-in.
		// Submitted invalid codes remain counted against the sign-in budget.
		if status.Code(err) != codes.InvalidArgument {
			attempt.succeeded()
		}
		return nil, err
	}
	attempt.succeeded()
	if loginIP != "" {
		blocked, err := s.Store.ClearLoginIPFailures(ctx, loginIP)
		if err != nil {
			return nil, status.Error(codes.Unavailable, "login protection is currently unavailable")
		}
		if blocked {
			return nil, blockedLoginIPError()
		}
	}
	accessToken, accessExpiresAt, err := s.doSignIn(ctx, existingUser, revision)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to sign in: %v", err)
	}

	return &apipb.SignInResponse{
		User:                  convertUserFromStore(existingUser, existingUser),
		AccessToken:           accessToken,
		AccessTokenExpireTime: timestamppb.New(accessExpiresAt),
	}, nil
}

// resolveSSOUser resolves a local user from an external-identity subject, creating the
// linkage record (and a new local user if necessary) when first login is allowed.
//
// Lookup goes through the user_identity table instead of using userInfo.Identifier
// as the local lookup key. On the miss path, a local user is created with the
// identifier as its username when valid and available, or a UUID fallback
// otherwise, and the (provider, extern_uid) linkage is committed atomically with
// the user. When currentUser is provided by a caller outside AuthService.SignIn,
// the lookup miss path binds the external identity to that existing user instead.
// Concurrent first logins reconcile uniqueness conflicts by loading the linkage
// winner.
