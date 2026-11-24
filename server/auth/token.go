// Package auth provides authentication and authorization for the Memos server.
//
// This package is used by:
// - server/api: gRPC and Connect API interceptors
// - server/fileserver: HTTP file server authentication
//
// Authentication methods supported:
// - JWT access tokens: Short-lived tokens (15 minutes) for API access
// - JWT refresh tokens: Long-lived tokens (30 days) for obtaining new access tokens
package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"

	"github.com/usememos/memos/store"
)

const (
	// Issuer is the issuer claim in JWT tokens.
	// This identifies tokens as issued by Memos.
	Issuer = "memos"

	// KeyID is the key identifier used in JWT header.
	// Version "v1" allows for future key rotation while maintaining backward compatibility.
	// If signing mechanism changes, add "v2", "v3", etc. and verify both versions.
	KeyID = "v1"

	// AccessTokenAudienceName is the audience claim for JWT access tokens.
	// This ensures tokens are only used for API access, not other purposes.
	AccessTokenAudienceName = "user.access-token"

	// AccessTokenDuration is the lifetime of access tokens (15 minutes).
	AccessTokenDuration = 15 * time.Minute

	// RefreshTokenDuration is the lifetime of refresh tokens (30 days).
	RefreshTokenDuration = 30 * 24 * time.Hour

	// RefreshTokenAudienceName is the audience claim for refresh tokens.
	RefreshTokenAudienceName = "user.refresh-token"

	// RefreshTokenCookieName is the cookie name for refresh tokens.
	RefreshTokenCookieName = "memos_refresh"
)

// ClaimsMessage represents the claims structure in a JWT token.
//
// JWT Claims include:
// - name: Username (custom claim)
// - iss: Issuer = "memos"
// - aud: Audience = "user.access-token"
// - sub: Subject = user ID
// - iat: Issued at time
// - exp: Expiration time (optional, may be empty for never-expiring tokens).
type ClaimsMessage struct {
	Name string `json:"name"` // Username
	jwt.RegisteredClaims
}

// AccessTokenClaims contains claims for short-lived access tokens.
// Authentication also checks AuthHash against the user's current password hash.
type AccessTokenClaims struct {
	Type            string `json:"type"`      // "access"
	Role            string `json:"role"`      // User role
	Status          string `json:"status"`    // User status
	Username        string `json:"username"`  // Username for display
	AuthHash        string `json:"auth_hash"` // Password-bound session validation
	SessionRevision string `json:"session_revision,omitempty"`
	SessionID       string `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

// RefreshTokenClaims contains claims for long-lived refresh tokens.
// These tokens are validated against the database for revocation.
type RefreshTokenClaims struct {
	Type            string `json:"type"` // "refresh"
	TokenID         string `json:"tid"`  // Token ID for revocation lookup
	SessionRevision string `json:"session_revision,omitempty"`
	SessionID       string `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

// StableSessionID uses the original refresh ID when upgrading a legacy session.
func (c *RefreshTokenClaims) StableSessionID() string {
	if c.SessionID != "" {
		return c.SessionID
	}
	return c.TokenID
}

// GenerateAccessToken generates a JWT access token for a user.
//
// Parameters:
// - username: The user's username (stored in "name" claim)
// - userID: The user's ID (stored in "sub" claim)
// - expirationTime: When the token expires (pass zero time for no expiration)
// - secret: Server secret used to sign the token
//
// Returns a signed JWT string or an error.
func GenerateAccessToken(username string, userID int32, expirationTime time.Time, secret []byte) (string, error) {
	return generateToken(username, userID, AccessTokenAudienceName, expirationTime, secret)
}

// generateToken generates a JWT token with the given claims.
//
// Token structure:
// Header: {"alg": "HS256", "kid": "v1", "typ": "JWT"}
// Claims: {"name": username, "iss": "memos", "aud": [audience], "sub": userID, "iat": now, "exp": expiry}
// Signature: HMACSHA256(base64UrlEncode(header) + "." + base64UrlEncode(payload), secret).
func generateToken(username string, userID int32, audience string, expirationTime time.Time, secret []byte) (string, error) {
	registeredClaims := jwt.RegisteredClaims{
		Issuer:   Issuer,
		Audience: jwt.ClaimStrings{audience},
		IssuedAt: jwt.NewNumericDate(time.Now()),
		Subject:  fmt.Sprint(userID),
	}
	if !expirationTime.IsZero() {
		registeredClaims.ExpiresAt = jwt.NewNumericDate(expirationTime)
	}

	// Declare the token with the HS256 algorithm used for signing, and the claims.
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &ClaimsMessage{
		Name:             username,
		RegisteredClaims: registeredClaims,
	})
	token.Header["kid"] = KeyID

	// Create the JWT string.
	tokenString, err := token.SignedString(secret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// GenerateAccessTokenV2 generates a short-lived access token bound to the user's current password hash.
func GenerateAccessTokenV2(userID int32, username, role, status, passwordHash string, secret []byte) (string, time.Time, error) {
	return issueAccessTokenV2(userID, username, role, status, passwordHash, "", "", secret)
}

// GenerateSessionAccessToken issues an access token for a proven factor revision.
func GenerateSessionAccessToken(user *store.User, sessionID, revision string, secret []byte) (string, time.Time, error) {
	return issueAccessTokenV2(user.ID, user.Username, string(user.Role), string(user.RowStatus), user.PasswordHash, sessionID, revision, secret)
}

func issueAccessTokenV2(userID int32, username, role, status, passwordHash, sessionID, revision string, secret []byte) (string, time.Time, error) {
	expiresAt := time.Now().Add(AccessTokenDuration)

	claims := &AccessTokenClaims{
		Type:            "access",
		Role:            role,
		Status:          status,
		Username:        username,
		AuthHash:        accessTokenAuthHash(userID, passwordHash, secret),
		SessionRevision: revision,
		SessionID:       sessionID,
		Issuer:          Issuer,
		Audience:        jwt.ClaimStrings{AccessTokenAudienceName},
		Subject:         fmt.Sprint(userID),
		IssuedAt:        jwt.NewNumericDate(time.Now()),
		ExpiresAt:       jwt.NewNumericDate(expiresAt),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = KeyID

	tokenString, err := token.SignedString(secret)
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expiresAt, nil
}

// accessTokenAuthHash follows the password-bound session hash pattern: changing
// the stored password invalidates every previously issued access token. The
// domain-separated HMAC hides the password hash and binds the value to a user.
func accessTokenAuthHash(userID int32, passwordHash string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("memos.access-token.auth-hash\x00" + fmt.Sprint(userID) + "\x00" + passwordHash))
	return hex.EncodeToString(mac.Sum(nil))
}

// GenerateRefreshToken generates a long-lived refresh token.
func GenerateRefreshToken(userID int32, tokenID string, secret []byte) (string, time.Time, error) {
	return GenerateSessionRefreshToken(userID, tokenID, "", "", secret)
}

// GenerateSessionRefreshToken issues a refresh token for a proven factor revision.
func GenerateSessionRefreshToken(userID int32, tokenID, sessionID, revision string, secret []byte) (string, time.Time, error) {
	expiresAt := time.Now().Add(RefreshTokenDuration)

	claims := &RefreshTokenClaims{
		Type:            "refresh",
		TokenID:         tokenID,
		SessionRevision: revision,
		SessionID:       sessionID,
		Issuer:          Issuer,
		Audience:        jwt.ClaimStrings{RefreshTokenAudienceName},
		Subject:         fmt.Sprint(userID),
		IssuedAt:        jwt.NewNumericDate(time.Now()),
		ExpiresAt:       jwt.NewNumericDate(expiresAt),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = KeyID

	tokenString, err := token.SignedString(secret)
	if err != nil {
		return "", time.Time{}, err
	}

	return tokenString, expiresAt, nil
}

// verifyJWTKeyFunc returns a jwt.Keyfunc that validates the signing method and key ID.
func verifyJWTKeyFunc(secret []byte) jwt.Keyfunc {
	return func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Name {
			return nil, errors.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		kid, ok := t.Header["kid"].(string)
		if !ok || kid != KeyID {
			return nil, errors.Errorf("unexpected kid: %v", t.Header["kid"])
		}
		return secret, nil
	}
}

// ParseAccessTokenV2 parses and validates a short-lived access token.
func ParseAccessTokenV2(tokenString string, secret []byte) (*AccessTokenClaims, error) {
	claims := &AccessTokenClaims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, verifyJWTKeyFunc(secret),
		jwt.WithIssuer(Issuer),
		jwt.WithAudience(AccessTokenAudienceName),
	)
	if err != nil {
		return nil, err
	}
	if claims.Type != "access" {
		return nil, errors.New("invalid token type: expected access token")
	}
	// Pre-upgrade access tokens cannot prove which password they were issued
	// against. Reject them; an existing refresh session can obtain a bound token.
	if claims.AuthHash == "" {
		return nil, errors.New("access token missing auth hash")
	}
	return claims, nil
}

// ParseRefreshToken parses and validates a refresh token.
func ParseRefreshToken(tokenString string, secret []byte) (*RefreshTokenClaims, error) {
	claims := &RefreshTokenClaims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, verifyJWTKeyFunc(secret),
		jwt.WithIssuer(Issuer),
		jwt.WithAudience(RefreshTokenAudienceName),
	)
	if err != nil {
		return nil, err
	}
	if claims.Type != "refresh" {
		return nil, errors.New("invalid token type: expected refresh token")
	}
	if claims.TokenID == "" {
		return nil, errors.New("refresh token missing token ID")
	}
	return claims, nil
}
