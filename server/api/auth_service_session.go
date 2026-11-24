package api

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/usememos/memos/internal/clientip"
	"github.com/usememos/memos/internal/random"
	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

const (
	refreshSessionIDHeader  = "x-memos-session-id"
	sessionIdleExpiryHeader = "X-Memos-Session-Idle-Expires-At"
)

func (s *APIService) doSignIn(ctx context.Context, user *store.User, revision string) (string, time.Time, error) {
	// Generate refresh token
	tokenID, sessionID := random.UUID(), random.UUID()
	refreshToken, refreshExpiresAt, err := auth.GenerateSessionRefreshToken(user.ID, tokenID, sessionID, revision, []byte(s.Secret))
	if err != nil {
		return "", time.Time{}, status.Errorf(codes.Internal, "failed to generate refresh token: %v", err)
	}
	accessToken, accessExpiresAt, err := auth.GenerateSessionAccessToken(user, sessionID, revision, []byte(s.Secret))
	if err != nil {
		return "", time.Time{}, status.Errorf(codes.Internal, "failed to generate access token: %v", err)
	}

	// Store refresh token metadata
	clientInfo := s.extractClientInfo(ctx)
	refreshTokenRecord := &storepb.RefreshTokensUserSetting_RefreshToken{
		TokenId:    tokenID,
		SessionId:  sessionID,
		ExpiresAt:  timestamppb.New(refreshExpiresAt),
		CreatedAt:  timestamppb.Now(),
		ClientInfo: clientInfo,
	}
	if s.personalMode() {
		refreshTokenRecord.LastEditAt = refreshTokenRecord.CreatedAt
	}
	if err := s.Store.AddUserRefreshTokenForSignIn(ctx, user, revision, refreshTokenRecord); err != nil {
		if errors.Is(err, store.ErrSignInCredentialsChanged) {
			return "", time.Time{}, status.Error(codes.Unauthenticated, "sign-in credentials changed, try again")
		}
		slog.Error("failed to store refresh token", "error", err)
		return "", time.Time{}, status.Error(codes.Internal, "failed to store refresh token")
	}

	// Set refresh token cookie
	refreshCookie := s.buildRefreshTokenCookie(ctx, refreshToken, refreshExpiresAt)
	if err := SetResponseHeader(ctx, "Set-Cookie", refreshCookie); err != nil {
		return "", time.Time{}, status.Errorf(codes.Internal, "failed to set refresh token cookie: %v", err)
	}
	if s.personalMode() {
		if err := setSessionIdleExpiryHeader(ctx, store.UserSessionIdleExpiresAt(refreshTokenRecord)); err != nil {
			return "", time.Time{}, err
		}
	}

	return accessToken, accessExpiresAt, nil
}

// SignOut revokes the session identified by the access token.
func (s *APIService) SignOut(ctx context.Context, _ *apipb.SignOutRequest) (*emptypb.Empty, error) {
	claims := auth.GetUserClaims(ctx)
	if claims == nil {
		return nil, status.Error(codes.PermissionDenied, "sign out requires an interactive session")
	}
	id := currentSessionID(ctx, s.Secret, claims.UserID)
	if id != "" {
		if err := s.Store.RemoveUserRefreshToken(ctx, claims.UserID, id); err != nil {
			slog.ErrorContext(ctx, "failed to revoke session on sign out", "error", err)
			return nil, status.Error(codes.Internal, "failed to revoke session")
		}
	}
	// Another browser session can hold the same-origin cookie. Clear it only
	// when it belongs to the session that just signed out.
	if cookie := refreshCookieClaims(ctx, s.Secret, claims.UserID); cookie != nil && cookie.StableSessionID() == id {
		if err := s.clearAuthCookies(ctx); err != nil {
			return nil, status.Errorf(codes.Internal, "failed to clear auth cookies: %v", err)
		}
	}
	return &emptypb.Empty{}, nil
}

// RefreshToken exchanges a valid refresh token for a new access token.
//
// This endpoint implements refresh token rotation with sliding window sessions:
// 1. Extracts the refresh token from the HttpOnly cookie (memos_refresh)
// 2. Validates the refresh token against the database (checking expiry and revocation)
// 3. Rotates the refresh token: generates a new one with fresh 30-day expiry
// 4. Generates a new short-lived access token (15 minutes)
// 5. Sets the new refresh token as HttpOnly cookie
// 6. Returns the new access token and its expiry time
//
// Token rotation provides:
// - Sliding window sessions: active users stay logged in indefinitely
// - Better security: stolen refresh tokens become invalid after legitimate refresh
//
// Authentication: Requires valid refresh token in cookie (public endpoint)
// Returns: New access token and expiry timestamp.
func (s *APIService) RefreshToken(ctx context.Context, _ *apipb.RefreshTokenRequest) (*apipb.RefreshTokenResponse, error) {
	refreshToken := refreshTokenFromContext(ctx)
	if refreshToken == "" {
		return nil, status.Errorf(codes.Unauthenticated, "refresh token not found")
	}

	// Validate refresh token and get old token ID for rotation
	authenticator := auth.NewAuthenticator(s.Store, s.Secret)
	user, oldSession, err := authenticator.AuthenticateRefreshSession(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, auth.ErrSessionIdleExpired) {
			return nil, status.Error(codes.Unauthenticated, auth.ErrSessionIdleExpired.Error())
		}
		if errors.Is(err, auth.ErrInvalidRefreshToken) {
			return nil, status.Error(codes.Unauthenticated, auth.ErrInvalidRefreshToken.Error())
		}
		slog.Error("failed to read refresh session", "error", err)
		return nil, status.Error(codes.Internal, "failed to read refresh session")
	}
	sessionID := oldSession.StableSessionID()
	if expected := metadata.ValueFromIncomingContext(ctx, refreshSessionIDHeader); len(expected) != 0 && expected[0] != sessionID {
		return nil, status.Error(codes.Unauthenticated, "refresh cookie belongs to a different session")
	}

	// --- Refresh Token Rotation ---
	// Generate new refresh token with fresh 30-day expiry (sliding window)
	newTokenID := random.UUID()
	newRefreshToken, newRefreshExpiresAt, err := auth.GenerateSessionRefreshToken(user.ID, newTokenID, sessionID, oldSession.SessionRevision, []byte(s.Secret))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate refresh token: %v", err)
	}

	accessToken, expiresAt, err := auth.GenerateSessionAccessToken(user, sessionID, oldSession.SessionRevision, []byte(s.Secret))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate access token: %v", err)
	}

	// Replace the old token in one write. A concurrent refresh or revocation
	// may already have removed it since AuthenticateRefreshSession read it.
	clientInfo := s.extractClientInfo(ctx)
	newRefreshTokenRecord := &storepb.RefreshTokensUserSetting_RefreshToken{
		TokenId:    newTokenID,
		SessionId:  sessionID,
		ExpiresAt:  timestamppb.New(newRefreshExpiresAt),
		CreatedAt:  timestamppb.Now(),
		ClientInfo: clientInfo,
	}
	rotated, idleExpiresAt, err := s.Store.RotateUserRefreshToken(ctx, user.ID, oldSession.TokenID, newRefreshTokenRecord)
	if err != nil {
		slog.Error("failed to rotate refresh token", "error", err)
		return nil, status.Error(codes.Internal, "failed to rotate refresh token")
	}
	if !rotated {
		return nil, status.Error(codes.Unauthenticated, "refresh token revoked")
	}

	// Set new refresh token cookie
	newRefreshCookie := s.buildRefreshTokenCookie(ctx, newRefreshToken, newRefreshExpiresAt)
	if err := SetResponseHeader(ctx, "Set-Cookie", newRefreshCookie); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to set refresh token cookie: %v", err)
	}
	if s.personalMode() {
		if err := setSessionIdleExpiryHeader(ctx, idleExpiresAt); err != nil {
			return nil, err
		}
	}
	// --- End Rotation ---

	return &apipb.RefreshTokenResponse{
		AccessToken:           accessToken,
		AccessTokenExpireTime: timestamppb.New(expiresAt),
	}, nil
}

func setSessionIdleExpiryHeader(ctx context.Context, deadline time.Time) error {
	if deadline.IsZero() {
		return nil
	}
	if err := SetResponseHeader(ctx, sessionIdleExpiryHeader, deadline.UTC().Format(time.RFC3339Nano)); err != nil {
		return status.Error(codes.Internal, "failed to set the session inactivity deadline")
	}
	return nil
}

func refreshTokenFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	cookies := md.Get("cookie")
	if len(cookies) == 0 {
		return ""
	}
	return auth.ExtractRefreshTokenFromCookie(cookies[0])
}

func refreshCookieClaims(ctx context.Context, secret string, userID int32) *auth.RefreshTokenClaims {
	refreshToken := refreshTokenFromContext(ctx)
	if refreshToken == "" {
		return nil
	}
	claims, err := auth.ParseRefreshToken(refreshToken, []byte(secret))
	if err != nil || claims.Subject != strconv.FormatInt(int64(userID), 10) {
		return nil
	}
	return claims
}

// currentSessionID identifies a session independently of refresh rotation.
// A legacy signed cookie can identify the session before it is upgraded.
func currentSessionID(ctx context.Context, secret string, userID int32) string {
	if claims := auth.GetUserClaims(ctx); claims != nil && claims.UserID == userID && claims.SessionID != "" {
		return claims.SessionID
	}
	if cookie := refreshCookieClaims(ctx, secret, userID); cookie != nil {
		return cookie.StableSessionID()
	}
	return ""
}

func (s *APIService) clearAuthCookies(ctx context.Context) error {
	// Clear refresh token cookie
	refreshCookie := s.buildRefreshTokenCookie(ctx, "", time.Time{})
	if err := SetResponseHeader(ctx, "Set-Cookie", refreshCookie); err != nil {
		return errors.Wrap(err, "failed to set refresh cookie")
	}

	return nil
}

func isSecureRequest(ctx context.Context) bool {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return false
	}

	for _, value := range md.Get("x-forwarded-proto") {
		for proto := range strings.SplitSeq(value, ",") {
			if strings.EqualFold(strings.TrimSpace(proto), "https") {
				return true
			}
		}
	}

	for _, value := range md.Get("forwarded") {
		lowerValue := strings.ToLower(value)
		if strings.Contains(lowerValue, "proto=https") {
			return true
		}
	}

	for _, value := range md.Get("origin") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "https://") {
			return true
		}
	}

	return false
}

func (s *APIService) buildRefreshTokenCookie(ctx context.Context, refreshToken string, expireTime time.Time) string {
	attrs := []string{
		fmt.Sprintf("%s=%s", auth.RefreshTokenCookieName, refreshToken),
		"Path=/",
		"HttpOnly",
	}
	if expireTime.IsZero() {
		attrs = append(attrs, "Expires=Thu, 01 Jan 1970 00:00:00 GMT")
	} else {
		// RFC 6265 requires cookie expiration dates to use GMT timezone
		// Convert to UTC and format with explicit "GMT" to ensure browser compatibility
		attrs = append(attrs, "Expires="+expireTime.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))
	}

	// The canonical HTTPS URL remains authoritative when TLS terminates at a
	// reverse proxy that omits forwarding headers.
	if (s.Profile != nil && strings.HasPrefix(s.Profile.InstanceURL, "https://")) || isSecureRequest(ctx) {
		attrs = append(attrs, "SameSite=Lax", "Secure")
	} else {
		attrs = append(attrs, "SameSite=Lax")
	}
	return strings.Join(attrs, "; ")
}

func (s *APIService) fetchCurrentUser(ctx context.Context) (*store.User, error) {
	userID := auth.GetUserID(ctx)
	if userID == 0 {
		return nil, nil
	}
	user, err := s.Store.GetUser(ctx, &store.FindUser{
		ID: &userID,
	})
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.Errorf("user %d not found", userID)
	}
	if !isActiveCurrentUser(user) {
		return nil, nil
	}
	return user, nil
}

func isActiveCurrentUser(user *store.User) bool {
	return user != nil && user.RowStatus == store.Normal
}

// extractClientInfo extracts comprehensive client information from the request context.
//
// This function parses metadata from the gRPC context to extract:
// - User Agent: Raw user agent string for detailed parsing
// - IP Address: Client IP as resolved through the trusted proxies
// - Device Type: "mobile", "tablet", or "desktop" (parsed from user agent)
// - Operating System: OS name and version (e.g., "iOS 17.1", "Windows 10/11")
// - Browser: Browser name and version (e.g., "Chrome 120.0.0.0")
//
// This information enables users to:
// - See all active sessions with device details
// - Identify suspicious login attempts
// - Revoke specific sessions from unknown devices.
func (s *APIService) extractClientInfo(ctx context.Context) *storepb.RefreshTokensUserSetting_ClientInfo {
	clientInfo := &storepb.RefreshTokensUserSetting_ClientInfo{}

	// Extract user agent from metadata if available
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if userAgents := md.Get("user-agent"); len(userAgents) > 0 {
			userAgent := userAgents[0]
			clientInfo.UserAgent = userAgent

			// Parse user agent to extract device type, OS, browser info
			s.parseUserAgent(userAgent, clientInfo)
		}
	}
	// The address comes from the trusted-proxy resolver, never from a header
	// the client chose.
	clientInfo.IpAddress = clientip.FromContext(ctx)

	return clientInfo
}

// parseUserAgent extracts device type, OS, and browser information from user agent string.
//
// Detection logic:
// - Device Type: Checks for keywords like "mobile", "tablet", "ipad"
// - OS: Pattern matches for iOS, Android, Windows, macOS, Linux, Chrome OS
// - Browser: Identifies Edge, Chrome, Firefox, Safari, Opera
//
// Note: This is a simplified parser. For production use with high accuracy requirements,
// consider using a dedicated user agent parsing library.
func (*APIService) parseUserAgent(userAgent string, clientInfo *storepb.RefreshTokensUserSetting_ClientInfo) {
	if userAgent == "" {
		return
	}

	userAgent = strings.ToLower(userAgent)

	// Detect device type
	if strings.Contains(userAgent, "ipad") || strings.Contains(userAgent, "tablet") {
		clientInfo.DeviceType = "tablet"
	} else if strings.Contains(userAgent, "mobile") || strings.Contains(userAgent, "android") ||
		strings.Contains(userAgent, "iphone") || strings.Contains(userAgent, "ipod") ||
		strings.Contains(userAgent, "windows phone") || strings.Contains(userAgent, "blackberry") {
		clientInfo.DeviceType = "mobile"
	} else {
		clientInfo.DeviceType = "desktop"
	}

	// Detect operating system
	if strings.Contains(userAgent, "iphone os") || strings.Contains(userAgent, "cpu os") {
		// Extract iOS version
		if idx := strings.Index(userAgent, "cpu os "); idx != -1 {
			versionStart := idx + 7
			versionEnd := strings.Index(userAgent[versionStart:], " ")
			if versionEnd != -1 {
				version := strings.ReplaceAll(userAgent[versionStart:versionStart+versionEnd], "_", ".")
				clientInfo.Os = "iOS " + version
			} else {
				clientInfo.Os = "iOS"
			}
		} else if idx := strings.Index(userAgent, "iphone os "); idx != -1 {
			versionStart := idx + 10
			versionEnd := strings.Index(userAgent[versionStart:], " ")
			if versionEnd != -1 {
				version := strings.ReplaceAll(userAgent[versionStart:versionStart+versionEnd], "_", ".")
				clientInfo.Os = "iOS " + version
			} else {
				clientInfo.Os = "iOS"
			}
		} else {
			clientInfo.Os = "iOS"
		}
	} else if strings.Contains(userAgent, "android") {
		// Extract Android version
		if idx := strings.Index(userAgent, "android "); idx != -1 {
			versionStart := idx + 8
			versionEnd := strings.Index(userAgent[versionStart:], ";")
			if versionEnd == -1 {
				versionEnd = strings.Index(userAgent[versionStart:], ")")
			}
			if versionEnd != -1 {
				version := userAgent[versionStart : versionStart+versionEnd]
				clientInfo.Os = "Android " + version
			} else {
				clientInfo.Os = "Android"
			}
		} else {
			clientInfo.Os = "Android"
		}
	} else if strings.Contains(userAgent, "windows nt 10.0") {
		clientInfo.Os = "Windows 10/11"
	} else if strings.Contains(userAgent, "windows nt 6.3") {
		clientInfo.Os = "Windows 8.1"
	} else if strings.Contains(userAgent, "windows nt 6.1") {
		clientInfo.Os = "Windows 7"
	} else if strings.Contains(userAgent, "windows") {
		clientInfo.Os = "Windows"
	} else if strings.Contains(userAgent, "mac os x") {
		// Extract macOS version
		if idx := strings.Index(userAgent, "mac os x "); idx != -1 {
			versionStart := idx + 9
			versionEnd := strings.Index(userAgent[versionStart:], ";")
			if versionEnd == -1 {
				versionEnd = strings.Index(userAgent[versionStart:], ")")
			}
			if versionEnd != -1 {
				version := strings.ReplaceAll(userAgent[versionStart:versionStart+versionEnd], "_", ".")
				clientInfo.Os = "macOS " + version
			} else {
				clientInfo.Os = "macOS"
			}
		} else {
			clientInfo.Os = "macOS"
		}
	} else if strings.Contains(userAgent, "linux") {
		clientInfo.Os = "Linux"
	} else if strings.Contains(userAgent, "cros") {
		clientInfo.Os = "Chrome OS"
	}

	// Detect browser
	if strings.Contains(userAgent, "edg/") {
		// Extract Edge version
		if idx := strings.Index(userAgent, "edg/"); idx != -1 {
			versionStart := idx + 4
			versionEnd := strings.Index(userAgent[versionStart:], " ")
			if versionEnd == -1 {
				versionEnd = len(userAgent) - versionStart
			}
			version := userAgent[versionStart : versionStart+versionEnd]
			clientInfo.Browser = "Edge " + version
		} else {
			clientInfo.Browser = "Edge"
		}
	} else if strings.Contains(userAgent, "chrome/") && !strings.Contains(userAgent, "edg") {
		// Extract Chrome version
		if idx := strings.Index(userAgent, "chrome/"); idx != -1 {
			versionStart := idx + 7
			versionEnd := strings.Index(userAgent[versionStart:], " ")
			if versionEnd == -1 {
				versionEnd = len(userAgent) - versionStart
			}
			version := userAgent[versionStart : versionStart+versionEnd]
			clientInfo.Browser = "Chrome " + version
		} else {
			clientInfo.Browser = "Chrome"
		}
	} else if strings.Contains(userAgent, "firefox/") {
		// Extract Firefox version
		if idx := strings.Index(userAgent, "firefox/"); idx != -1 {
			versionStart := idx + 8
			versionEnd := strings.Index(userAgent[versionStart:], " ")
			if versionEnd == -1 {
				versionEnd = len(userAgent) - versionStart
			}
			version := userAgent[versionStart : versionStart+versionEnd]
			clientInfo.Browser = "Firefox " + version
		} else {
			clientInfo.Browser = "Firefox"
		}
	} else if strings.Contains(userAgent, "safari/") && !strings.Contains(userAgent, "chrome") && !strings.Contains(userAgent, "edg") {
		// Extract Safari version
		if idx := strings.Index(userAgent, "version/"); idx != -1 {
			versionStart := idx + 8
			versionEnd := strings.Index(userAgent[versionStart:], " ")
			if versionEnd == -1 {
				versionEnd = len(userAgent) - versionStart
			}
			version := userAgent[versionStart : versionStart+versionEnd]
			clientInfo.Browser = "Safari " + version
		} else {
			clientInfo.Browser = "Safari"
		}
	} else if strings.Contains(userAgent, "opera/") || strings.Contains(userAgent, "opr/") {
		clientInfo.Browser = "Opera"
	}
}
