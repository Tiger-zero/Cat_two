package test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/usememos/memos/internal/profile"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

type pausedSessionDriver struct {
	store.Driver
	ready, proceed chan struct{}
	pause          sync.Once
}

func (driver *pausedSessionDriver) CompareAndSetUserSetting(ctx context.Context, setting *store.UserSetting, previous *string) (bool, error) {
	first := false
	driver.pause.Do(func() {
		first = true
		close(driver.ready)
	})
	if first {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-driver.proceed:
		}
	}
	return driver.Driver.CompareAndSetUserSetting(ctx, setting, previous)
}

func (*pausedSessionDriver) Close() error { return nil }

func TestUserSettingRefreshCannotRestoreSessionRevokedByAnotherStore(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	const sessionID = "stable-session"
	require.NoError(t, ts.AddUserRefreshToken(ctx, user.ID, &storepb.RefreshTokensUserSetting_RefreshToken{
		TokenId: "first-token", SessionId: sessionID,
	}))
	paused := &pausedSessionDriver{Driver: ts.GetDriver(), ready: make(chan struct{}), proceed: make(chan struct{})}
	otherStore := store.New(paused, &profile.Profile{Data: ts.GetDataDir()})
	defer otherStore.Close()
	type outcome struct {
		changed bool
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		changed, _, err := otherStore.RotateUserRefreshToken(ctx, user.ID, "first-token", &storepb.RefreshTokensUserSetting_RefreshToken{
			TokenId: "second-token", SessionId: sessionID,
		})
		done <- outcome{changed: changed, err: err}
	}()
	select {
	case <-ctx.Done():
		t.Fatal("refresh did not reach the database write")
	case <-paused.ready:
	}
	require.NoError(t, ts.RemoveUserRefreshToken(ctx, user.ID, sessionID))
	close(paused.proceed)
	select {
	case <-ctx.Done():
		t.Fatal("refresh did not stop after revocation")
	case result := <-done:
		require.NoError(t, result.err)
		require.False(t, result.changed)
	}
	active, _, err := ts.IsUserSessionActive(ctx, user.ID, sessionID)
	require.NoError(t, err)
	require.False(t, active)
}

func TestUserSettingRefreshPreservesEditCommittedByAnotherStore(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	const sessionID = "edited-session"
	created := time.Now().Add(-5 * time.Minute).UTC()
	require.NoError(t, ts.AddUserRefreshToken(ctx, user.ID, &storepb.RefreshTokensUserSetting_RefreshToken{
		TokenId: "first-token", SessionId: sessionID, CreatedAt: timestamppb.New(created),
	}))
	paused := &pausedSessionDriver{Driver: ts.GetDriver(), ready: make(chan struct{}), proceed: make(chan struct{})}
	otherStore := store.New(paused, &profile.Profile{Data: ts.GetDataDir(), Personal: true})
	defer otherStore.Close()
	type outcome struct {
		changed  bool
		deadline time.Time
		err      error
	}
	done := make(chan outcome, 1)
	go func() {
		changed, deadline, err := otherStore.RotateUserRefreshToken(ctx, user.ID, "first-token", &storepb.RefreshTokensUserSetting_RefreshToken{
			TokenId: "rotated-token", SessionId: sessionID, CreatedAt: timestamppb.Now(),
		})
		done <- outcome{changed: changed, deadline: deadline, err: err}
	}()
	select {
	case <-ctx.Done():
		t.Fatal("refresh did not reach the database write")
	case <-paused.ready:
	}
	deadline, err := ts.RecordUserSessionEdit(ctx, user.ID, sessionID)
	require.NoError(t, err)
	require.True(t, deadline.After(created.Add(store.UserSessionIdleTimeout)))
	close(paused.proceed)
	select {
	case <-ctx.Done():
		t.Fatal("refresh did not retry after the edit")
	case result := <-done:
		require.NoError(t, result.err)
		require.True(t, result.changed)
		require.Equal(t, deadline, result.deadline)
	}
	stored, err := ts.GetUserRefreshTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, "rotated-token", stored[0].GetTokenId())
	require.Equal(t, deadline, store.UserSessionIdleExpiresAt(stored[0]))
}

func TestUserSettingRefreshTokensPruneExpiredOnAdd(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	expired := &storepb.RefreshTokensUserSetting_RefreshToken{TokenId: "expired", ExpiresAt: timestamppb.New(time.Now().Add(-time.Hour))}
	live := &storepb.RefreshTokensUserSetting_RefreshToken{TokenId: "live", ExpiresAt: timestamppb.New(time.Now().Add(time.Hour))}
	require.NoError(t, ts.AddUserRefreshToken(ctx, user.ID, expired))
	require.NoError(t, ts.AddUserRefreshToken(ctx, user.ID, live))
	require.NoError(t, ts.AddUserRefreshToken(ctx, user.ID, &storepb.RefreshTokensUserSetting_RefreshToken{TokenId: "newest"}))

	tokens, err := ts.GetUserRefreshTokens(ctx, user.ID)
	require.NoError(t, err)
	ids := []string{}
	for _, token := range tokens {
		ids = append(ids, token.TokenId)
	}
	require.Equal(t, []string{"live", "newest"}, ids)
}

func TestUserSettingRemoveRefreshTokensExcept(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	for _, id := range []string{"a", "b", "c"} {
		require.NoError(t, ts.AddUserRefreshToken(ctx, user.ID, &storepb.RefreshTokensUserSetting_RefreshToken{TokenId: id}))
	}

	require.NoError(t, ts.RemoveUserRefreshTokensExcept(ctx, user.ID, "b"))
	tokens, err := ts.GetUserRefreshTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.Equal(t, "b", tokens[0].TokenId)

	require.NoError(t, ts.RemoveUserRefreshTokensExcept(ctx, user.ID, ""))
	tokens, err = ts.GetUserRefreshTokens(ctx, user.ID)
	require.NoError(t, err)
	require.Empty(t, tokens)
}
