package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/usememos/memos/internal/profile"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
	storedb "github.com/usememos/memos/store/db"
)

// TestUpgradeFromCalendarRelease uses local store fixtures to verify that
// schema versions advance independently of application versions.
func TestUpgradeFromCalendarRelease(t *testing.T) {
	for _, tc := range []struct{ previous, candidate string }{
		{previous: "26.09.0", candidate: "26.10.0"},
		{previous: "26.10.0", candidate: "26.10.1"},
		{previous: "26.10.0-rc.1", candidate: "26.10.0"},
		{previous: "26.12.9", candidate: "27.01.0"},
	} {
		t.Run(tc.previous+"-to-"+tc.candidate, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			previousProfile := getTestingProfileForDriver(t, getDriverFromEnv())
			previousProfile.Version = tc.previous
			previousDriver, err := storedb.NewDBDriver(previousProfile)
			require.NoError(t, err)
			previous := store.New(previousDriver, previousProfile)
			t.Cleanup(func() { _ = previous.Close() })
			require.NoError(t, previous.Migrate(ctx))
			baseline, err := previous.GetInstanceBasicSetting(ctx)
			require.NoError(t, err)
			currentSchema, err := previous.GetCurrentSchemaVersion()
			require.NoError(t, err)
			require.Equal(t, currentSchema, baseline.SchemaVersion, "application CalVer must not become the database schema version")

			candidateProfile := *previousProfile
			candidateProfile.Version = tc.candidate
			requireUpgradePreservesData(ctx, t, previous, &candidateProfile)
		})
	}
}

// requireUpgradePreservesData closes the source store and opens the same
// database with the candidate profile, then checks upgrade and restart behavior.
func requireUpgradePreservesData(ctx context.Context, t *testing.T, previous *store.Store, candidateProfile *profile.Profile) {
	t.Helper()
	user, err := createTestingHostUser(ctx, previous)
	require.NoError(t, err)
	memo, err := previous.CreateMemo(ctx, &store.Memo{
		UID: "upgrade-sentinel", CreatorID: user.ID, Content: "#upgrade\n- [ ] preserved task", Visibility: store.Private,
		Payload: &storepb.MemoPayload{Tags: []string{"upgrade"}},
	})
	require.NoError(t, err)
	memo.Pinned = true
	require.NoError(t, previous.UpdateMemo(ctx, &store.UpdateMemo{ID: memo.ID, Pinned: &memo.Pinned, UpdatedTs: &memo.UpdatedTs}))
	memo, err = previous.GetMemo(ctx, &store.FindMemo{ID: &memo.ID})
	require.NoError(t, err)
	const legacyView = `{"memoViews":[{"id":"upgrade","title":"Upgrade sentinel","filter":"has_incomplete_tasks"}]}`
	insertView := "INSERT INTO user_setting (user_id, `key`, `value`) VALUES (?, 'MEMO_VIEWS', ?)"
	readView := "SELECT `value` FROM user_setting WHERE user_id = ? AND `key` = 'MEMO_VIEWS'"
	if candidateProfile.Driver == "postgres" {
		insertView = "INSERT INTO user_setting (user_id, key, value) VALUES ($1, 'MEMO_VIEWS', $2)"
		readView = "SELECT value FROM user_setting WHERE user_id = $1 AND key = 'MEMO_VIEWS'"
	}
	_, err = previous.GetDriver().GetDB().ExecContext(ctx, insertView, user.ID, legacyView)
	require.NoError(t, err)
	editor, err := previous.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: user.ID, Key: storepb.UserSetting_EDITOR,
		Value: &storepb.UserSetting_Editor{Editor: &storepb.EditorUserSetting{SourceMode: true}},
	})
	require.NoError(t, err)
	access, err := previous.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_ACCESS,
		Value: &storepb.InstanceSetting_AccessSetting{AccessSetting: &storepb.InstanceAccessSetting{
			AccessMode: storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PUBLIC,
		}},
	})
	require.NoError(t, err)
	require.NoError(t, previous.Close())

	verify := func(candidate *store.Store) {
		t.Helper()
		currentSchema, err := candidate.GetCurrentSchemaVersion()
		require.NoError(t, err)
		basic, err := candidate.GetInstanceBasicSetting(ctx)
		require.NoError(t, err)
		require.Equal(t, currentSchema, basic.SchemaVersion)
		preservedUser, err := candidate.GetUser(ctx, &store.FindUser{ID: &user.ID})
		require.NoError(t, err)
		require.Equal(t, user, preservedUser)
		preservedMemo, err := candidate.GetMemo(ctx, &store.FindMemo{UID: &memo.UID})
		require.NoError(t, err)
		require.EqualExportedValues(t, memo, preservedMemo)
		var preservedView string
		err = candidate.GetDriver().GetDB().QueryRowContext(ctx, readView, user.ID).Scan(&preservedView)
		require.NoError(t, err)
		require.Equal(t, legacyView, preservedView, "legacy saved view data must survive the upgrade")
		settings, err := candidate.ListUserSettings(ctx, &store.FindUserSetting{UserID: &user.ID})
		require.NoError(t, err)
		require.Len(t, settings, 1, "retired views must not appear in current settings")
		require.True(t, proto.Equal(editor, settings[0]), "editor preferences must survive the upgrade")
		preservedAccess, err := candidate.GetInstanceAccessSetting(ctx)
		require.NoError(t, err)
		require.True(t, proto.Equal(access.GetAccessSetting(), preservedAccess), "explicit access mode must not be reset")
		requireLookupIndexes(ctx, t, candidate, candidateProfile.Driver)
	}

	driver, err := storedb.NewDBDriver(candidateProfile)
	require.NoError(t, err)
	candidate := store.New(driver, candidateProfile)
	t.Cleanup(func() { _ = candidate.Close() })
	require.NoError(t, candidate.Migrate(ctx))
	verify(candidate)
	written, err := candidate.CreateMemo(ctx, &store.Memo{
		UID: "after-upgrade", CreatorID: user.ID, Content: "created after upgrade", Visibility: store.Private,
	})
	require.NoError(t, err, "upgraded database must remain writable")
	require.NoError(t, candidate.Close())

	restartedDriver, err := storedb.NewDBDriver(candidateProfile)
	require.NoError(t, err)
	restarted := store.New(restartedDriver, candidateProfile)
	t.Cleanup(func() { _ = restarted.Close() })
	require.NoError(t, restarted.Migrate(ctx), "restart after upgrade must be idempotent")
	verify(restarted)
	persisted, err := restarted.GetMemo(ctx, &store.FindMemo{UID: &written.UID})
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.Equal(t, written.Content, persisted.Content, "post-upgrade writes must survive restart")
}
