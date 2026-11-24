package test

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/usememos/memos/internal/random"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

func TestAuthenticatorAccessTokenV2(t *testing.T) {
	ctx := context.Background()

	t.Run("authenticates valid access token v2", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a test user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Generate access token v2
		token, _, err := auth.GenerateAccessTokenV2(
			user.ID,
			user.Username,
			string(user.Role),
			string(user.RowStatus),
			user.PasswordHash,
			[]byte(ts.Secret),
		)
		require.NoError(t, err)

		// Authenticate
		authenticator := auth.NewAuthenticator(ts.Store, ts.Secret)
		claims, err := authenticator.AuthenticateByAccessTokenV2(ctx, token)
		require.NoError(t, err)
		assert.NotNil(t, claims)
		assert.Equal(t, user.ID, claims.UserID)
		assert.Equal(t, user.Username, claims.Username)
		assert.Equal(t, string(user.Role), claims.Role)
		assert.Equal(t, string(user.RowStatus), claims.Status)
	})

	t.Run("fails with invalid token", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		authenticator := auth.NewAuthenticator(ts.Store, ts.Secret)
		_, err := authenticator.AuthenticateByAccessTokenV2(ctx, "invalid-token")
		assert.Error(t, err)
	})

	t.Run("fails with wrong secret", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Generate token with one secret
		token, _, err := auth.GenerateAccessTokenV2(
			user.ID,
			user.Username,
			string(user.Role),
			string(user.RowStatus),
			user.PasswordHash,
			[]byte("secret-1"),
		)
		require.NoError(t, err)

		// Try to authenticate with different secret
		authenticator := auth.NewAuthenticator(ts.Store, "secret-2")
		_, err = authenticator.AuthenticateByAccessTokenV2(ctx, token)
		assert.Error(t, err)
	})

	t.Run("request authentication rejects archived user", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "archived-access-token")
		require.NoError(t, err)
		token, _, err := auth.GenerateAccessTokenV2(
			user.ID,
			user.Username,
			string(user.Role),
			string(user.RowStatus),
			user.PasswordHash,
			[]byte(ts.Secret),
		)
		require.NoError(t, err)

		archivedStatus := store.Archived
		_, err = ts.Store.UpdateUser(ctx, &store.UpdateUser{
			ID:        user.ID,
			RowStatus: &archivedStatus,
		})
		require.NoError(t, err)

		authenticator := auth.NewAuthenticator(ts.Store, ts.Secret)
		result := authenticator.Authenticate(ctx, "Bearer "+token)
		assert.Nil(t, result)
	})

	t.Run("rejects missing or mismatched password binding", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "password-binding")
		require.NoError(t, err)
		token, _, err := auth.GenerateAccessTokenV2(user.ID, user.Username, string(user.Role), string(user.RowStatus), user.PasswordHash, []byte(ts.Secret))
		require.NoError(t, err)
		claims, err := auth.ParseAccessTokenV2(token, []byte(ts.Secret))
		require.NoError(t, err)
		authenticator := auth.NewAuthenticator(ts.Store, ts.Secret)
		for _, authHash := range []string{"", "wrong-password-binding"} {
			claims.AuthHash = authHash
			jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			jwtToken.Header["kid"] = auth.KeyID
			unboundToken, err := jwtToken.SignedString([]byte(ts.Secret))
			require.NoError(t, err)
			_, err = authenticator.AuthenticateByAccessTokenV2(ctx, unboundToken)
			require.Error(t, err)
			require.Nil(t, authenticator.Authenticate(ctx, "Bearer "+unboundToken))
		}
	})

	t.Run("profile changes preserve access tokens", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()
		user, err := ts.CreateRegularUser(ctx, "profile-change")
		require.NoError(t, err)
		token, _, err := auth.GenerateAccessTokenV2(user.ID, user.Username, string(user.Role), string(user.RowStatus), user.PasswordHash, []byte(ts.Secret))
		require.NoError(t, err)
		nickname := "New display name"
		_, err = ts.Store.UpdateUser(ctx, &store.UpdateUser{ID: user.ID, Nickname: &nickname})
		require.NoError(t, err)
		require.NotNil(t, auth.NewAuthenticator(ts.Store, ts.Secret).Authenticate(ctx, "Bearer "+token))
	})
}

func TestAuthenticatorRefreshToken(t *testing.T) {
	ctx := context.Background()

	t.Run("authenticates valid refresh token", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		// Create a test user
		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Create refresh token record in store
		tokenID := random.UUID()
		refreshTokenRecord := &storepb.RefreshTokensUserSetting_RefreshToken{
			TokenId:   tokenID,
			ExpiresAt: timestamppb.New(time.Now().Add(auth.RefreshTokenDuration)),
			CreatedAt: timestamppb.Now(),
		}
		err = ts.Store.AddUserRefreshToken(ctx, user.ID, refreshTokenRecord)
		require.NoError(t, err)

		// Generate refresh token JWT
		token, _, err := auth.GenerateRefreshToken(user.ID, tokenID, []byte(ts.Secret))
		require.NoError(t, err)

		// Authenticate
		authenticator := auth.NewAuthenticator(ts.Store, ts.Secret)
		authenticatedUser, returnedTokenID, err := authenticator.AuthenticateByRefreshToken(ctx, token)
		require.NoError(t, err)
		assert.NotNil(t, authenticatedUser)
		assert.Equal(t, user.ID, authenticatedUser.ID)
		assert.Equal(t, tokenID, returnedTokenID)
	})

	t.Run("fails with revoked token", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		tokenID := random.UUID()

		// Generate refresh token JWT but don't store it in database (simulates revocation)
		token, _, err := auth.GenerateRefreshToken(user.ID, tokenID, []byte(ts.Secret))
		require.NoError(t, err)

		// Try to authenticate
		authenticator := auth.NewAuthenticator(ts.Store, ts.Secret)
		_, _, err = authenticator.AuthenticateByRefreshToken(ctx, token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "revoked")
	})

	t.Run("fails with expired token", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Create expired refresh token record in store
		tokenID := random.UUID()
		expiredToken := &storepb.RefreshTokensUserSetting_RefreshToken{
			TokenId:   tokenID,
			ExpiresAt: timestamppb.New(time.Now().Add(-1 * time.Hour)), // Expired
			CreatedAt: timestamppb.Now(),
		}
		err = ts.Store.AddUserRefreshToken(ctx, user.ID, expiredToken)
		require.NoError(t, err)

		// Generate refresh token JWT (JWT itself isn't expired yet)
		token, _, err := auth.GenerateRefreshToken(user.ID, tokenID, []byte(ts.Secret))
		require.NoError(t, err)

		// Try to authenticate
		authenticator := auth.NewAuthenticator(ts.Store, ts.Secret)
		_, _, err = authenticator.AuthenticateByRefreshToken(ctx, token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expired")
	})

	t.Run("fails with archived user", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Create valid refresh token
		tokenID := random.UUID()
		refreshTokenRecord := &storepb.RefreshTokensUserSetting_RefreshToken{
			TokenId:   tokenID,
			ExpiresAt: timestamppb.New(time.Now().Add(auth.RefreshTokenDuration)),
			CreatedAt: timestamppb.Now(),
		}
		err = ts.Store.AddUserRefreshToken(ctx, user.ID, refreshTokenRecord)
		require.NoError(t, err)

		token, _, err := auth.GenerateRefreshToken(user.ID, tokenID, []byte(ts.Secret))
		require.NoError(t, err)

		// Archive the user
		archivedStatus := store.Archived
		_, err = ts.Store.UpdateUser(ctx, &store.UpdateUser{
			ID:        user.ID,
			RowStatus: &archivedStatus,
		})
		require.NoError(t, err)

		// Try to authenticate
		authenticator := auth.NewAuthenticator(ts.Store, ts.Secret)
		_, _, err = authenticator.AuthenticateByRefreshToken(ctx, token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "archived")
	})
}

func TestStoreRefreshTokenMethods(t *testing.T) {
	ctx := context.Background()

	t.Run("adds and retrieves refresh token", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		tokenID := random.UUID()
		token := &storepb.RefreshTokensUserSetting_RefreshToken{
			TokenId:   tokenID,
			ExpiresAt: timestamppb.New(time.Now().Add(30 * 24 * time.Hour)),
			CreatedAt: timestamppb.Now(),
		}

		err = ts.Store.AddUserRefreshToken(ctx, user.ID, token)
		require.NoError(t, err)

		// Retrieve tokens
		tokens, err := ts.Store.GetUserRefreshTokens(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, tokens, 1)
		assert.Equal(t, tokenID, tokens[0].TokenId)
	})

	t.Run("retrieves specific refresh token by ID", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		tokenID := random.UUID()
		token := &storepb.RefreshTokensUserSetting_RefreshToken{
			TokenId:   tokenID,
			ExpiresAt: timestamppb.New(time.Now().Add(30 * 24 * time.Hour)),
			CreatedAt: timestamppb.Now(),
		}

		err = ts.Store.AddUserRefreshToken(ctx, user.ID, token)
		require.NoError(t, err)

		// Retrieve specific token
		retrievedToken, err := ts.Store.GetUserRefreshTokenByID(ctx, user.ID, tokenID)
		require.NoError(t, err)
		assert.NotNil(t, retrievedToken)
		assert.Equal(t, tokenID, retrievedToken.TokenId)
	})

	t.Run("removes refresh token", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		tokenID := random.UUID()
		token := &storepb.RefreshTokensUserSetting_RefreshToken{
			TokenId:   tokenID,
			ExpiresAt: timestamppb.New(time.Now().Add(30 * 24 * time.Hour)),
			CreatedAt: timestamppb.Now(),
		}

		err = ts.Store.AddUserRefreshToken(ctx, user.ID, token)
		require.NoError(t, err)

		// Remove token
		err = ts.Store.RemoveUserRefreshToken(ctx, user.ID, tokenID)
		require.NoError(t, err)

		// Verify removal
		tokens, err := ts.Store.GetUserRefreshTokens(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, tokens, 0)
	})

	t.Run("handles multiple refresh tokens", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "testuser")
		require.NoError(t, err)

		// Add multiple tokens
		tokenID1 := random.UUID()
		tokenID2 := random.UUID()

		token1 := &storepb.RefreshTokensUserSetting_RefreshToken{
			TokenId:   tokenID1,
			ExpiresAt: timestamppb.New(time.Now().Add(30 * 24 * time.Hour)),
			CreatedAt: timestamppb.Now(),
		}
		token2 := &storepb.RefreshTokensUserSetting_RefreshToken{
			TokenId:   tokenID2,
			ExpiresAt: timestamppb.New(time.Now().Add(30 * 24 * time.Hour)),
			CreatedAt: timestamppb.Now(),
		}

		err = ts.Store.AddUserRefreshToken(ctx, user.ID, token1)
		require.NoError(t, err)
		err = ts.Store.AddUserRefreshToken(ctx, user.ID, token2)
		require.NoError(t, err)

		// Retrieve all tokens
		tokens, err := ts.Store.GetUserRefreshTokens(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, tokens, 2)

		// Remove one token
		err = ts.Store.RemoveUserRefreshToken(ctx, user.ID, tokenID1)
		require.NoError(t, err)

		// Verify only one token remains
		tokens, err = ts.Store.GetUserRefreshTokens(ctx, user.ID)
		require.NoError(t, err)
		assert.Len(t, tokens, 1)
		assert.Equal(t, tokenID2, tokens[0].TokenId)
	})
}
