package test

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

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

	// List all settings for user
	settings, err := ts.ListUserSettings(ctx, &store.FindUserSetting{UserID: &user.ID})
	require.NoError(t, err)
	require.Len(t, settings, 2)

	// Verify each setting type
	generalSetting, err := ts.GetUserSetting(ctx, &store.FindUserSetting{UserID: &user.ID, Key: storepb.UserSetting_GENERAL})
	require.NoError(t, err)
	require.Equal(t, "ja", generalSetting.GetGeneral().Locale)

	editorSetting, err := ts.GetUserSetting(ctx, &store.FindUserSetting{UserID: &user.ID, Key: storepb.UserSetting_EDITOR})
	require.NoError(t, err)
	require.True(t, editorSetting.GetEditor().GetSourceMode())

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
