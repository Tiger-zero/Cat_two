package test

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

func TestUserSettingStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	_, err = ts.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: user.ID,
		Key:    storepb.UserSetting_GENERAL,
		Value:  &storepb.UserSetting_General{General: &storepb.GeneralUserSetting{Locale: "en"}},
	})
	require.NoError(t, err)
	list, err := ts.ListUserSettings(ctx, &store.FindUserSetting{})
	require.NoError(t, err)
	require.Equal(t, 1, len(list))
	ts.Close()
}

func TestUserSettingGetByUserID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create setting
	_, err = ts.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: user.ID,
		Key:    storepb.UserSetting_GENERAL,
		Value:  &storepb.UserSetting_General{General: &storepb.GeneralUserSetting{Locale: "zh"}},
	})
	require.NoError(t, err)

	// Get by user ID
	setting, err := ts.GetUserSetting(ctx, &store.FindUserSetting{
		UserID: &user.ID,
		Key:    storepb.UserSetting_GENERAL,
	})
	require.NoError(t, err)
	require.NotNil(t, setting)
	require.Equal(t, "zh", setting.GetGeneral().Locale)

	// Get non-existent key
	nonExistentSetting, err := ts.GetUserSetting(ctx, &store.FindUserSetting{
		UserID: &user.ID,
		Key:    storepb.UserSetting_EDITOR,
	})
	require.NoError(t, err)
	require.Nil(t, nonExistentSetting)

	ts.Close()
}

func TestUserSettingUpsertUpdate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create initial setting
	_, err = ts.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: user.ID,
		Key:    storepb.UserSetting_GENERAL,
		Value:  &storepb.UserSetting_General{General: &storepb.GeneralUserSetting{Locale: "en"}},
	})
	require.NoError(t, err)

	// Update setting
	_, err = ts.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: user.ID,
		Key:    storepb.UserSetting_GENERAL,
		Value:  &storepb.UserSetting_General{General: &storepb.GeneralUserSetting{Locale: "fr"}},
	})
	require.NoError(t, err)

	// Verify update
	setting, err := ts.GetUserSetting(ctx, &store.FindUserSetting{
		UserID: &user.ID,
		Key:    storepb.UserSetting_GENERAL,
	})
	require.NoError(t, err)
	require.Equal(t, "fr", setting.GetGeneral().Locale)

	// Verify only one setting exists
	list, err := ts.ListUserSettings(ctx, &store.FindUserSetting{UserID: &user.ID})
	require.NoError(t, err)
	require.Equal(t, 1, len(list))

	ts.Close()
}

func TestUserSettingRefreshTokens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Initially no tokens
	tokens, err := ts.GetUserRefreshTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Empty(t, tokens)

	// Add a refresh token
	token1 := &storepb.RefreshTokensUserSetting_RefreshToken{
		TokenId:     "token-1",
		Description: "Chrome browser session",
	}
	err = ts.AddUserRefreshToken(ctx, user.ID, token1)
	require.NoError(t, err)

	// Verify token was added
	tokens, err = ts.GetUserRefreshTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.Equal(t, "token-1", tokens[0].TokenId)

	// Add another token
	token2 := &storepb.RefreshTokensUserSetting_RefreshToken{
		TokenId:     "token-2",
		Description: "Firefox browser session",
	}
	err = ts.AddUserRefreshToken(ctx, user.ID, token2)
	require.NoError(t, err)

	tokens, err = ts.GetUserRefreshTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, tokens, 2)

	// Get specific token by ID
	foundToken, err := ts.GetUserRefreshTokenByID(ctx, user.ID, "token-1")
	require.NoError(t, err)
	require.NotNil(t, foundToken)
	require.Equal(t, "Chrome browser session", foundToken.Description)

	// Get non-existent token
	notFound, err := ts.GetUserRefreshTokenByID(ctx, user.ID, "non-existent")
	require.NoError(t, err)
	require.Nil(t, notFound)

	// Remove token
	err = ts.RemoveUserRefreshToken(ctx, user.ID, "token-1")
	require.NoError(t, err)

	tokens, err = ts.GetUserRefreshTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.Equal(t, "token-2", tokens[0].TokenId)

	ts.Close()
}

func TestUserSettingRefreshTokensConcurrentAdds(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	const tokenCount = 16
	start := make(chan struct{})
	errCh := make(chan error, tokenCount)
	var wg sync.WaitGroup
	for i := range tokenCount {
		tokenID := strconv.Itoa(i)
		wg.Go(func() {
			<-start
			errCh <- ts.AddUserRefreshToken(ctx, user.ID, &storepb.RefreshTokensUserSetting_RefreshToken{TokenId: tokenID})
		})
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	tokens, err := ts.GetUserRefreshTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, tokens, tokenCount)
	seen := make(map[string]bool, tokenCount)
	for _, token := range tokens {
		require.False(t, seen[token.TokenId], "duplicate token %q", token.TokenId)
		seen[token.TokenId] = true
	}
}

func TestUserSettingPersonalAccessTokens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Initially no PATs
	pats, err := ts.GetUserPersonalAccessTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Empty(t, pats)

	// Add a PAT
	pat1 := &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-1",
		TokenHash:   "pat-hash-1",
		Description: "API Token for external access",
	}
	err = ts.AddUserPersonalAccessToken(ctx, user.ID, pat1)
	require.NoError(t, err)

	// Verify PAT was added
	pats, err = ts.GetUserPersonalAccessTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, pats, 1)
	require.Equal(t, "API Token for external access", pats[0].Description)

	// Add another PAT
	pat2 := &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-2",
		TokenHash:   "pat-hash-2",
		Description: "CI Token",
	}
	err = ts.AddUserPersonalAccessToken(ctx, user.ID, pat2)
	require.NoError(t, err)

	pats, err = ts.GetUserPersonalAccessTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, pats, 2)

	// Remove PAT
	err = ts.RemoveUserPersonalAccessToken(ctx, user.ID, "pat-1")
	require.NoError(t, err)

	pats, err = ts.GetUserPersonalAccessTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, pats, 1)
	require.Equal(t, "pat-2", pats[0].TokenId)

	ts.Close()
}

func TestUserSettingWebhooks(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Initially no webhooks
	webhooks, err := ts.GetUserWebhooks(ctx, user.ID)
	require.NoError(t, err)
	require.Empty(t, webhooks)

	// Add a webhook
	webhook1 := &storepb.WebhooksUserSetting_Webhook{
		Id:    "webhook-1",
		Title: "Deploy Hook",
		Url:   "https://example.com/webhook",
	}
	err = ts.AddUserWebhook(ctx, user.ID, webhook1)
	require.NoError(t, err)

	// Verify webhook was added
	webhooks, err = ts.GetUserWebhooks(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, webhooks, 1)
	require.Equal(t, "Deploy Hook", webhooks[0].Title)

	// Update webhook
	webhook1Updated := &storepb.WebhooksUserSetting_Webhook{
		Id:    "webhook-1",
		Title: "Updated Deploy Hook",
		Url:   "https://example.com/webhook/v2",
	}
	err = ts.UpdateUserWebhook(ctx, user.ID, webhook1Updated)
	require.NoError(t, err)

	webhooks, err = ts.GetUserWebhooks(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, webhooks, 1)
	require.Equal(t, "Updated Deploy Hook", webhooks[0].Title)
	require.Equal(t, "https://example.com/webhook/v2", webhooks[0].Url)

	// Add another webhook
	webhook2 := &storepb.WebhooksUserSetting_Webhook{
		Id:    "webhook-2",
		Title: "Notification Hook",
		Url:   "https://slack.example.com/webhook",
	}
	err = ts.AddUserWebhook(ctx, user.ID, webhook2)
	require.NoError(t, err)

	webhooks, err = ts.GetUserWebhooks(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, webhooks, 2)

	// Remove webhook
	err = ts.RemoveUserWebhook(ctx, user.ID, "webhook-1")
	require.NoError(t, err)

	webhooks, err = ts.GetUserWebhooks(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, webhooks, 1)
	require.Equal(t, "webhook-2", webhooks[0].Id)

	ts.Close()
}

func TestUserSettingGetUserByPATHash(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create a PAT with a known hash
	patHash := "test-pat-hash-12345"
	pat := &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-test-1",
		TokenHash:   patHash,
		Description: "Test PAT for lookup",
	}
	err = ts.AddUserPersonalAccessToken(ctx, user.ID, pat)
	require.NoError(t, err)

	// Lookup user by PAT hash
	result, err := ts.GetUserByPATHash(ctx, patHash)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, user.ID, result.UserID)
	require.NotNil(t, result.User)
	require.Equal(t, user.Username, result.User.Username)
	require.NotNil(t, result.PAT)
	require.Equal(t, "pat-test-1", result.PAT.TokenId)
	require.Equal(t, "Test PAT for lookup", result.PAT.Description)

	ts.Close()
}

func TestUserSettingGetUserByPATHashNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	_, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Lookup non-existent PAT hash
	result, err := ts.GetUserByPATHash(ctx, "non-existent-hash")
	require.NoError(t, err)
	require.Nil(t, result)

	ts.Close()
}

func TestUserSettingGetUserByPATHashNoTokensKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// User exists but has no PERSONAL_ACCESS_TOKENS key at all
	// This simulates fresh users or users upgraded from v0.25.3
	result, err := ts.GetUserByPATHash(ctx, "any-hash")
	require.NoError(t, err)
	require.Nil(t, result)

	// Now add a PAT for the user
	pat := &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-new",
		TokenHash:   "hash-new",
		Description: "New PAT",
	}
	err = ts.AddUserPersonalAccessToken(ctx, user.ID, pat)
	require.NoError(t, err)

	// Now the lookup should succeed
	result, err = ts.GetUserByPATHash(ctx, "hash-new")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, user.ID, result.UserID)

	ts.Close()
}

func TestUserSettingGetUserByPATHashEmptyTokensArray(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Add a PAT setting with empty tokens array
	_, err = ts.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: user.ID,
		Key:    storepb.UserSetting_PERSONAL_ACCESS_TOKENS,
		Value: &storepb.UserSetting_PersonalAccessTokens{
			PersonalAccessTokens: &storepb.PersonalAccessTokensUserSetting{
				Tokens: []*storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{},
			},
		},
	})
	require.NoError(t, err)

	// Lookup should fail gracefully, not crash
	result, err := ts.GetUserByPATHash(ctx, "any-hash")
	require.NoError(t, err)
	require.Nil(t, result)

	ts.Close()
}

func TestUserSettingGetUserByPATHashWithOtherUsers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)

	// Create multiple users - some with PATs, some without
	user1, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	_, err = createTestingUserWithRole(ctx, ts, "user2", store.RoleUser)
	require.NoError(t, err)

	user3, err := createTestingUserWithRole(ctx, ts, "user3", store.RoleUser)
	require.NoError(t, err)

	// User1: Has PAT
	pat1Hash := "user1-pat-hash-unique"
	err = ts.AddUserPersonalAccessToken(ctx, user1.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-user1",
		TokenHash:   pat1Hash,
		Description: "User 1 PAT",
	})
	require.NoError(t, err)

	// User2: Has no PERSONAL_ACCESS_TOKENS key (fresh user)
	// User3: Has empty tokens array
	_, err = ts.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: user3.ID,
		Key:    storepb.UserSetting_PERSONAL_ACCESS_TOKENS,
		Value: &storepb.UserSetting_PersonalAccessTokens{
			PersonalAccessTokens: &storepb.PersonalAccessTokensUserSetting{
				Tokens: []*storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{},
			},
		},
	})
	require.NoError(t, err)

	// Should find user1's PAT despite user2 having no key and user3 having empty array
	result, err := ts.GetUserByPATHash(ctx, pat1Hash)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, user1.ID, result.UserID)
	require.Equal(t, "pat-user1", result.PAT.TokenId)

	// Should not find non-existent hash even with mixed user states
	result, err = ts.GetUserByPATHash(ctx, "non-existent")
	require.NoError(t, err)
	require.Nil(t, result)

	ts.Close()
}

func TestUserSettingGetUserByPATHashMultipleUsers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user1, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	user2, err := createTestingUserWithRole(ctx, ts, "user2", store.RoleUser)
	require.NoError(t, err)

	// Create PATs for both users
	pat1Hash := "user1-pat-hash"
	err = ts.AddUserPersonalAccessToken(ctx, user1.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-user1",
		TokenHash:   pat1Hash,
		Description: "User 1 PAT",
	})
	require.NoError(t, err)

	pat2Hash := "user2-pat-hash"
	err = ts.AddUserPersonalAccessToken(ctx, user2.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-user2",
		TokenHash:   pat2Hash,
		Description: "User 2 PAT",
	})
	require.NoError(t, err)

	// Lookup user1's PAT
	result1, err := ts.GetUserByPATHash(ctx, pat1Hash)
	require.NoError(t, err)
	require.Equal(t, user1.ID, result1.UserID)
	require.Equal(t, user1.Username, result1.User.Username)

	// Lookup user2's PAT
	result2, err := ts.GetUserByPATHash(ctx, pat2Hash)
	require.NoError(t, err)
	require.Equal(t, user2.ID, result2.UserID)
	require.Equal(t, user2.Username, result2.User.Username)

	ts.Close()
}

func TestUserSettingGetUserByPATHashMultiplePATsSameUser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create multiple PATs for the same user
	pat1Hash := "first-pat-hash"
	err = ts.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-1",
		TokenHash:   pat1Hash,
		Description: "First PAT",
	})
	require.NoError(t, err)

	pat2Hash := "second-pat-hash"
	err = ts.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-2",
		TokenHash:   pat2Hash,
		Description: "Second PAT",
	})
	require.NoError(t, err)

	// Both PATs should resolve to the same user
	result1, err := ts.GetUserByPATHash(ctx, pat1Hash)
	require.NoError(t, err)
	require.Equal(t, user.ID, result1.UserID)
	require.Equal(t, "pat-1", result1.PAT.TokenId)

	result2, err := ts.GetUserByPATHash(ctx, pat2Hash)
	require.NoError(t, err)
	require.Equal(t, user.ID, result2.UserID)
	require.Equal(t, "pat-2", result2.PAT.TokenId)

	ts.Close()
}

func TestUserSettingUpdatePATLastUsed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create a PAT
	patHash := "pat-hash-for-update"
	err = ts.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-update-test",
		TokenHash:   patHash,
		Description: "PAT for update test",
	})
	require.NoError(t, err)

	// Update last used timestamp
	now := timestamppb.Now()
	err = ts.UpdatePATLastUsed(ctx, user.ID, "pat-update-test", now)
	require.NoError(t, err)

	// Verify the update
	pats, err := ts.GetUserPersonalAccessTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, pats, 1)
	require.NotNil(t, pats[0].LastUsedAt)
	require.Equal(t, now.AsTime(), pats[0].LastUsedAt.AsTime())

	// An older asynchronous update must not make the last-used time regress.
	err = ts.UpdatePATLastUsed(ctx, user.ID, "pat-update-test", timestamppb.New(now.AsTime().Add(-time.Hour)))
	require.NoError(t, err)
	pats, err = ts.GetUserPersonalAccessTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, now.AsTime(), pats[0].LastUsedAt.AsTime())

	ts.Close()
}

func TestUserSettingUpdatePATLastUsedKeepsRevokedTokenRemoved(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	require.NoError(t, ts.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId: "revoked-pat-id", TokenHash: "revoked-pat-hash",
	}))
	key := storepb.UserSetting_PERSONAL_ACCESS_TOKENS
	_, err = ts.GetUserSetting(ctx, &store.FindUserSetting{UserID: &user.ID, Key: key})
	require.NoError(t, err)

	// Remove the row directly to exercise the credential read with an older
	// snapshot still in the general settings cache.
	require.NoError(t, ts.GetDriver().DeleteUserSettings(ctx, &store.DeleteUserSetting{UserID: &user.ID, Key: key}))
	require.NoError(t, ts.UpdatePATLastUsed(ctx, user.ID, "revoked-pat-id", timestamppb.Now()))
	result, err := ts.GetUserByPATHash(ctx, "revoked-pat-hash")
	require.NoError(t, err)
	require.Nil(t, result)
}

func TestUserSettingGetUserByPATHashWithExpiredToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create a PAT with expiration info
	patHash := "pat-hash-with-expiry"
	expiresAt := timestamppb.Now()
	pat := &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-expiry-test",
		TokenHash:   patHash,
		Description: "PAT with expiry",
		ExpiresAt:   expiresAt,
	}
	err = ts.AddUserPersonalAccessToken(ctx, user.ID, pat)
	require.NoError(t, err)

	// Should still be able to look up by hash (expiry check is done at auth level)
	result, err := ts.GetUserByPATHash(ctx, patHash)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, user.ID, result.UserID)
	require.NotNil(t, result.PAT.ExpiresAt)

	ts.Close()
}

func TestUserSettingGetUserByPATHashAfterRemoval(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create a PAT
	patHash := "pat-hash-to-remove"
	err = ts.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-remove-test",
		TokenHash:   patHash,
		Description: "PAT to be removed",
	})
	require.NoError(t, err)

	// Verify it exists
	result, err := ts.GetUserByPATHash(ctx, patHash)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Remove the PAT
	err = ts.RemoveUserPersonalAccessToken(ctx, user.ID, "pat-remove-test")
	require.NoError(t, err)

	// Should no longer be found
	result, err = ts.GetUserByPATHash(ctx, patHash)
	require.NoError(t, err)
	require.Nil(t, result)

	ts.Close()
}

func TestUserSettingGetUserByPATHashSpecialCharacters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create PATs with special characters in hash (simulating real hash values)
	testCases := []struct {
		tokenID   string
		tokenHash string
	}{
		{"pat-special-1", "abc123+/=XYZ"},
		{"pat-special-2", "sha256:abcdef1234567890"},
		{"pat-special-3", "$2a$10$N9qo8uLOickgx2ZMRZoMy"},
	}

	for _, tc := range testCases {
		err = ts.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
			TokenId:     tc.tokenID,
			TokenHash:   tc.tokenHash,
			Description: "PAT with special chars",
		})
		require.NoError(t, err)

		// Verify lookup works with special characters
		result, err := ts.GetUserByPATHash(ctx, tc.tokenHash)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, tc.tokenID, result.PAT.TokenId)
	}

	ts.Close()
}

func TestUserSettingGetUserByPATHashLargeTokenCount(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create many PATs for the same user
	tokenCount := 10
	hashes := make([]string, tokenCount)
	for i := range tokenCount {
		hashes[i] = "pat-hash-" + string(rune('A'+i)) + "-large-test"
		err = ts.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
			TokenId:     "pat-large-" + string(rune('A'+i)),
			TokenHash:   hashes[i],
			Description: "PAT for large count test",
		})
		require.NoError(t, err)
	}

	// Verify each hash can be looked up
	for i, hash := range hashes {
		result, err := ts.GetUserByPATHash(ctx, hash)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, user.ID, result.UserID)
		require.Equal(t, "pat-large-"+string(rune('A'+i)), result.PAT.TokenId)
	}

	ts.Close()
}

func TestUserSettingGetUserByPATHashCorruptRow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	other, err := createTestingUserWithRole(ctx, ts, "pat-corrupt-other", store.RoleUser)
	require.NoError(t, err)

	// A stored row the server cannot decode (lastUsedAt is not a timestamp).
	const corruptHash = "pat-hash-corrupt"
	_, err = ts.GetDriver().UpsertUserSetting(ctx, &store.UserSetting{
		UserID: user.ID,
		Key:    storepb.UserSetting_PERSONAL_ACCESS_TOKENS,
		Value:  `{"tokens":[{"tokenId":"pat-corrupt","tokenHash":"` + corruptHash + `","lastUsedAt":""}]}`,
	})
	require.NoError(t, err)

	// Looking up the corrupt token is a lookup failure, not a miss.
	result, err := ts.GetUserByPATHash(ctx, corruptHash)
	require.Error(t, err)
	require.Nil(t, result)

	// Other users' tokens still resolve.
	const otherHash = "pat-hash-other-healthy"
	err = ts.AddUserPersonalAccessToken(ctx, other.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:   "pat-other",
		TokenHash: otherHash,
	})
	require.NoError(t, err)
	result, err = ts.GetUserByPATHash(ctx, otherHash)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, other.ID, result.UserID)

	// An unknown hash is still a plain miss.
	result, err = ts.GetUserByPATHash(ctx, "pat-hash-unknown")
	require.NoError(t, err)
	require.Nil(t, result)

	ts.Close()
}

func TestUserSettingMultipleSettingTypes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Create GENERAL setting
	_, err = ts.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: user.ID,
		Key:    storepb.UserSetting_GENERAL,
		Value:  &storepb.UserSetting_General{General: &storepb.GeneralUserSetting{Locale: "ja"}},
	})
	require.NoError(t, err)

	// Create EDITOR setting
	_, err = ts.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: user.ID,
		Key:    storepb.UserSetting_EDITOR,
		Value:  &storepb.UserSetting_Editor{Editor: &storepb.EditorUserSetting{SourceMode: true}},
	})
	require.NoError(t, err)

	// Add a PAT
	err = ts.AddUserPersonalAccessToken(ctx, user.ID, &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:   "pat-multi",
		TokenHash: "hash-multi",
	})
	require.NoError(t, err)

	// List all settings for user
	settings, err := ts.ListUserSettings(ctx, &store.FindUserSetting{UserID: &user.ID})
	require.NoError(t, err)
	require.Len(t, settings, 3)

	// Verify each setting type
	generalSetting, err := ts.GetUserSetting(ctx, &store.FindUserSetting{UserID: &user.ID, Key: storepb.UserSetting_GENERAL})
	require.NoError(t, err)
	require.Equal(t, "ja", generalSetting.GetGeneral().Locale)

	editorSetting, err := ts.GetUserSetting(ctx, &store.FindUserSetting{UserID: &user.ID, Key: storepb.UserSetting_EDITOR})
	require.NoError(t, err)
	require.True(t, editorSetting.GetEditor().GetSourceMode())

	patsSetting, err := ts.GetUserSetting(ctx, &store.FindUserSetting{UserID: &user.ID, Key: storepb.UserSetting_PERSONAL_ACCESS_TOKENS})
	require.NoError(t, err)
	require.Len(t, patsSetting.GetPersonalAccessTokens().Tokens, 1)

	ts.Close()
}

func TestUserSettingEditor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	_, err = ts.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: user.ID,
		Key:    storepb.UserSetting_EDITOR,
		Value:  &storepb.UserSetting_Editor{Editor: &storepb.EditorUserSetting{SourceMode: true}},
	})
	require.NoError(t, err)

	setting, err := ts.GetUserSetting(ctx, &store.FindUserSetting{UserID: &user.ID, Key: storepb.UserSetting_EDITOR})
	require.NoError(t, err)
	require.NotNil(t, setting)
	require.True(t, setting.GetEditor().GetSourceMode())

	_, err = ts.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: user.ID,
		Key:    storepb.UserSetting_EDITOR,
		Value:  &storepb.UserSetting_Editor{Editor: &storepb.EditorUserSetting{SourceMode: false}},
	})
	require.NoError(t, err)

	setting, err = ts.GetUserSetting(ctx, &store.FindUserSetting{UserID: &user.ID, Key: storepb.UserSetting_EDITOR})
	require.NoError(t, err)
	require.NotNil(t, setting)
	require.False(t, setting.GetEditor().GetSourceMode())

	ts.Close()
}

func TestUserSettingJSONFieldsEdgeCases(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	// Case 1: Webhook with special characters and Unicode in Title and URL
	specialWebhook := &storepb.WebhooksUserSetting_Webhook{
		Id:    "wh-special",
		Title: `My "Special" & <Webhook> 🚀`,
		Url:   "https://example.com/hook?query=你好&param=\"value\"",
	}
	err = ts.AddUserWebhook(ctx, user.ID, specialWebhook)
	require.NoError(t, err)

	webhooks, err := ts.GetUserWebhooks(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, webhooks, 1)
	require.Equal(t, specialWebhook.Title, webhooks[0].Title)
	require.Equal(t, specialWebhook.Url, webhooks[0].Url)

	// Case 2: PAT with special description
	specialPAT := &storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{
		TokenId:     "pat-special",
		TokenHash:   "hash-special",
		Description: "Token for 'CLI' \n & \"API\" \t with unicode 🔑",
	}
	err = ts.AddUserPersonalAccessToken(ctx, user.ID, specialPAT)
	require.NoError(t, err)

	pats, err := ts.GetUserPersonalAccessTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, pats, 1)
	require.Equal(t, specialPAT.Description, pats[0].Description)

	// Case 3: Refresh Token with special description
	specialRefreshToken := &storepb.RefreshTokensUserSetting_RefreshToken{
		TokenId:     "rt-special",
		Description: "Browser: Firefox (Nightly) / OS: Linux 🐧",
	}
	err = ts.AddUserRefreshToken(ctx, user.ID, specialRefreshToken)
	require.NoError(t, err)

	tokens, err := ts.GetUserRefreshTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.Equal(t, specialRefreshToken.Description, tokens[0].Description)

	ts.Close()
}
