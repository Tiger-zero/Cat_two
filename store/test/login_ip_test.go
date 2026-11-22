package test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
	storedb "github.com/usememos/memos/store/db"
)

func TestLoginIPFailuresResetAndBlock(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	const ip = "192.0.2.10"
	const mappedIP = "::ffff:192.0.2.10"
	const limit = 3

	count, blocked, err := st.RecordLoginIPFailure(ctx, mappedIP, limit)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.False(t, blocked)
	removed, err := st.UnblockLoginIP(ctx, ip)
	require.NoError(t, err)
	require.False(t, removed, "manual removal addresses only permanent blocks")
	count, blocked, err = st.RecordLoginIPFailure(ctx, ip, limit)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.False(t, blocked)
	blocked, err = st.ClearLoginIPFailures(ctx, ip)
	require.NoError(t, err)
	require.False(t, blocked)

	for attempt := 1; attempt <= limit; attempt++ {
		count, blocked, err = st.RecordLoginIPFailure(ctx, ip, limit)
		require.NoError(t, err)
		require.Equal(t, attempt, count)
		require.Equal(t, attempt == limit, blocked)
	}
	blocks, err := st.ListBlockedLoginIPs(ctx, "", 10)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	require.Equal(t, ip, blocks[0].IP)
	require.Equal(t, limit, blocks[0].FailureCount)
	require.WithinDuration(t, time.Now(), blocks[0].BlockedAt, time.Minute)

	blocked, err = st.ClearLoginIPFailures(ctx, mappedIP)
	require.NoError(t, err)
	require.True(t, blocked, "successful sign-in cannot erase a permanent block")
	count, blocked, err = st.RecordLoginIPFailure(ctx, ip, limit)
	require.NoError(t, err)
	require.Equal(t, limit, count)
	require.True(t, blocked)
	after, err := st.ListBlockedLoginIPs(ctx, "", 10)
	require.NoError(t, err)
	require.Equal(t, blocks, after, "consecutive failures freeze once the IP is blocked")
	removed, err = st.UnblockLoginIP(ctx, mappedIP)
	require.NoError(t, err)
	require.True(t, removed)
	blocked, err = st.IsLoginIPBlocked(ctx, ip)
	require.NoError(t, err)
	require.False(t, blocked)
	count, blocked, err = st.RecordLoginIPFailure(ctx, ip, limit)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.False(t, blocked)
}

func TestLoginIPConcurrentFailuresReachLimitOnce(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, st.Close()) })
	const ip = "2001:db8::7"
	const limit = 5
	const attempts = 12
	start := make(chan struct{})
	type observation struct {
		count   int
		blocked bool
		err     error
	}
	results := make(chan observation, attempts)
	for range attempts {
		go func() {
			<-start
			count, blocked, err := st.RecordLoginIPFailure(ctx, ip, limit)
			results <- observation{count, blocked, err}
		}()
	}
	close(start)
	counts := make(map[int]int, limit)
	for range attempts {
		result := <-results
		require.NoError(t, result.err)
		require.Equal(t, result.count == limit, result.blocked)
		counts[result.count]++
	}
	for count := 1; count < limit; count++ {
		require.Equal(t, 1, counts[count])
	}
	require.Equal(t, attempts-limit+1, counts[limit])
}

func TestLoginIPBlockedListUsesCanonicalIPCursor(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, st.Close()) })

	for _, ip := range []string{"2001:0DB8:0:0::2", "::ffff:192.0.2.10", "192.0.2.2"} {
		_, blocked, err := st.RecordLoginIPFailure(ctx, ip, 1)
		require.NoError(t, err)
		require.True(t, blocked)
	}
	_, blocked, err := st.RecordLoginIPFailure(ctx, "192.0.2.1", 2)
	require.NoError(t, err)
	require.False(t, blocked)
	firstPage, err := st.ListBlockedLoginIPs(ctx, "", 2)
	require.NoError(t, err)
	require.Len(t, firstPage, 2)
	require.Equal(t, "192.0.2.10", firstPage[0].IP)
	require.Equal(t, "192.0.2.2", firstPage[1].IP)
	secondPage, err := st.ListBlockedLoginIPs(ctx, firstPage[1].IP, 2)
	require.NoError(t, err)
	require.Len(t, secondPage, 1)
	require.Equal(t, "2001:db8::2", secondPage[0].IP)
}

func TestLoginIPBlocksSurviveSQLiteMigrationAndRestart(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	p := getTestingProfileForDriver(t, "sqlite")
	openStore := func() *store.Store {
		t.Helper()
		driver, err := storedb.NewDBDriver(p)
		require.NoError(t, err)
		st := store.New(driver, p)
		t.Cleanup(func() { _ = st.Close() })
		require.NoError(t, st.Migrate(ctx))
		return st
	}

	// Recreate the previous schema state by dropping only this migration's
	// table and resetting the schema marker to the preceding migration.
	st := openStore()
	_, err := st.GetDriver().GetDB().ExecContext(ctx, "DROP TABLE login_ip_attempt")
	require.NoError(t, err)
	removeMemoBodySchema(ctx, t, st, "sqlite")
	basic, err := st.GetInstanceBasicSetting(ctx)
	require.NoError(t, err)
	_, err = st.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_BASIC,
		Value: &storepb.InstanceSetting_BasicSetting{BasicSetting: &storepb.InstanceBasicSetting{
			SchemaVersion: "26.11.6", SecretKey: basic.GetSecretKey(),
		}},
	})
	require.NoError(t, err)
	require.NoError(t, st.Close())

	st = openStore()
	const ip = "fe80::42"
	const pendingIP = "2001:db8::16"
	_, blocked, err := st.RecordLoginIPFailure(ctx, ip+"%eth0", 1)
	require.NoError(t, err)
	require.True(t, blocked)
	count, blocked, err := st.RecordLoginIPFailure(ctx, pendingIP, 2)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.False(t, blocked)
	require.NoError(t, st.Close())

	st = openStore()
	blocked, err = st.IsLoginIPBlocked(ctx, ip)
	require.NoError(t, err)
	require.True(t, blocked)
	count, blocked, err = st.RecordLoginIPFailure(ctx, pendingIP, 2)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.True(t, blocked)
}
