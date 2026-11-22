package test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestMCPConnectionStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	owner, err := createTestingUserWithRole(ctx, ts, "mcp-owner", store.RoleUser)
	require.NoError(t, err)
	other, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)

	connection := &store.MCPConnection{
		ID: "first", UserID: owner.ID, Label: strings.Repeat("📝", 80), TokenHash: strings.Repeat("ab", 32),
		Status: store.MCPConnectionApproved, CreatedAt: 100, ExpiresAt: 200, Permission: store.MCPConnectionReadOnly,
	}
	require.NoError(t, ts.CreateMCPConnection(ctx, connection))
	duplicate := *connection
	duplicate.ID, duplicate.UserID = "duplicate-hash", other.ID
	require.Error(t, ts.CreateMCPConnection(ctx, &duplicate))
	foreign := &store.MCPConnection{
		ID: "foreign", UserID: other.ID, Label: "Other", TokenHash: strings.Repeat("bc", 32),
		Status: store.MCPConnectionApproved, CreatedAt: 101,
	}
	require.NoError(t, ts.CreateMCPConnection(ctx, foreign))
	require.Equal(t, store.MCPConnectionReadWrite, foreign.Permission)

	list, err := ts.ListMCPConnections(ctx, owner.ID)
	require.NoError(t, err)
	require.Equal(t, []*store.MCPConnection{connection}, list)
	encoded, err := json.Marshal(list[0])
	require.NoError(t, err)
	require.NotContains(t, string(encoded), connection.TokenHash)
	require.NotContains(t, string(encoded), "userId")
	for _, absentHash := range []string{strings.ToUpper(connection.TokenHash), strings.Repeat("01", 32)} {
		stored, ownerStatus, err := ts.GetMCPConnectionByHash(ctx, absentHash)
		require.NoError(t, err)
		require.Nil(t, stored)
		require.Empty(t, ownerStatus)
	}
	stored, ownerStatus, err := ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Equal(t, connection, stored)
	require.Equal(t, owner.ID, stored.UserID)
	require.Equal(t, store.Normal, ownerStatus)
	stored, ownerStatus, err = ts.GetMCPConnectionByHash(ctx, foreign.TokenHash)
	require.NoError(t, err)
	require.Equal(t, foreign, stored)
	require.Equal(t, other.ID, stored.UserID)
	require.Equal(t, store.Normal, ownerStatus)

	require.NoError(t, ts.RecordMCPConnectionFirstUse(ctx, connection.TokenHash, 110, "192.0.2.1"))
	require.NoError(t, ts.RecordMCPConnectionFirstUse(ctx, connection.TokenHash, 120, "192.0.2.2"))
	stored, ownerStatus, err = ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Equal(t, store.Normal, ownerStatus)
	require.Equal(t, store.MCPConnectionApproved, stored.Status)
	require.EqualValues(t, 110, stored.RequestedAt)
	require.Equal(t, "192.0.2.1", stored.RemoteIP)
	require.EqualValues(t, 200, stored.ExpiresAt)
	beforeArchive := *stored

	// Change the row through the driver so the lookup cannot rely on the
	// cached user snapshot in this Store.
	archived := store.Archived
	_, err = ts.GetDriver().UpdateUser(ctx, &store.UpdateUser{ID: owner.ID, RowStatus: &archived})
	require.NoError(t, err)
	cachedOwner, err := ts.GetUser(ctx, &store.FindUser{ID: &owner.ID})
	require.NoError(t, err)
	require.Equal(t, store.Normal, cachedOwner.RowStatus)
	stored, ownerStatus, err = ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Equal(t, store.Archived, ownerStatus)
	require.Equal(t, &beforeArchive, stored)
	stored, ownerStatus, err = ts.GetMCPConnectionByHash(ctx, foreign.TokenHash)
	require.NoError(t, err)
	require.Equal(t, foreign, stored)
	require.Equal(t, store.Normal, ownerStatus)

	updated, err := ts.UpdateMCPConnection(ctx, other.ID, connection.ID, store.MCPConnectionApproved, store.MCPConnectionRevoked, 0)
	require.NoError(t, err)
	require.False(t, updated)

	start := make(chan struct{})
	type change struct {
		status  store.MCPConnectionStatus
		updated bool
		err     error
	}
	results := make(chan change, 2)
	for _, status := range []store.MCPConnectionStatus{store.MCPConnectionRevoked, store.MCPConnectionDenied} {
		go func() {
			<-start
			updated, err := ts.UpdateMCPConnection(ctx, owner.ID, connection.ID, store.MCPConnectionApproved, status, 0)
			results <- change{status: status, updated: updated, err: err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.NotEqual(t, first.updated, second.updated, "exactly one concurrent transition may succeed")
	winner := first
	if second.updated {
		winner = second
	}
	require.NoError(t, ts.RecordMCPConnectionFirstUse(ctx, connection.TokenHash, 130, "2001:db8::1"))
	stored, ownerStatus, err = ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Equal(t, store.Archived, ownerStatus)
	require.Equal(t, winner.status, stored.Status)
	require.Zero(t, stored.ExpiresAt)
	require.EqualValues(t, 110, stored.RequestedAt)
	require.Equal(t, "192.0.2.1", stored.RemoteIP)

	_, err = ts.DeleteUser(ctx, &store.DeleteUser{ID: owner.ID})
	require.NoError(t, err)
	stored, ownerStatus, err = ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Nil(t, stored)
	require.Empty(t, ownerStatus)
	require.Error(t, ts.CreateMCPConnection(ctx, connection), "a deleted account cannot own another connection")
	stored, ownerStatus, err = ts.GetMCPConnectionByHash(ctx, foreign.TokenHash)
	require.NoError(t, err)
	require.Equal(t, foreign, stored)
	require.Equal(t, store.Normal, ownerStatus)
}

func TestMCPConnectionFirstUseSkipsInactiveStates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	owner, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	for _, state := range []store.MCPConnectionStatus{
		store.MCPConnectionAwaiting, store.MCPConnectionPending, store.MCPConnectionDenied, store.MCPConnectionRevoked,
	} {
		t.Run(string(state), func(t *testing.T) {
			tokenHash := "inactive-" + string(state)
			connection := &store.MCPConnection{
				ID: string(state), UserID: owner.ID, TokenHash: tokenHash, Label: "Historical connection",
				Status: state, CreatedAt: 100,
			}
			require.NoError(t, ts.CreateMCPConnection(ctx, connection))
			require.NoError(t, ts.RecordMCPConnectionFirstUse(ctx, tokenHash, 110, "192.0.2.1"))
			stored, ownerStatus, err := ts.GetMCPConnectionByHash(ctx, tokenHash)
			require.NoError(t, err)
			require.Equal(t, connection, stored)
			require.Equal(t, store.Normal, ownerStatus)
		})
	}
}

func TestDeleteMCPConnectionOwnerScope(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	owner, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	other, err := createTestingUserWithRole(ctx, ts, "other-mcp-owner", store.RoleUser)
	require.NoError(t, err)
	connection := &store.MCPConnection{
		ID: "remove-owned", UserID: owner.ID, Label: "Owned connection", TokenHash: strings.Repeat("de", 32),
		Status: store.MCPConnectionApproved, CreatedAt: 100,
	}
	require.NoError(t, ts.CreateMCPConnection(ctx, connection))

	removed, err := ts.DeleteMCPConnection(ctx, other.ID, connection.ID)
	require.NoError(t, err)
	require.False(t, removed)
	stored, ownerStatus, err := ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Equal(t, connection, stored)
	require.Equal(t, store.Normal, ownerStatus)

	removed, err = ts.DeleteMCPConnection(ctx, owner.ID, connection.ID)
	require.NoError(t, err)
	require.True(t, removed)
	stored, ownerStatus, err = ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Nil(t, stored)
	require.Empty(t, ownerStatus)
	list, err := ts.ListMCPConnections(ctx, owner.ID)
	require.NoError(t, err)
	require.Empty(t, list)
	removed, err = ts.DeleteMCPConnection(ctx, owner.ID, connection.ID)
	require.NoError(t, err)
	require.False(t, removed)
}

func TestMigrationMCPConnection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	owner, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	removeSchemaAfterMCPConnection(ctx, t, ts, getDriverFromEnv())
	_, err = ts.GetDriver().GetDB().ExecContext(ctx, "DROP TABLE mcp_connection")
	require.NoError(t, err)
	setSchemaVersion(ctx, t, ts, "26.11.3")
	require.NoError(t, ts.Migrate(ctx))
	require.NoError(t, ts.Migrate(ctx), "a completed migration can restart")
	setting, err := ts.GetUncachedInstanceSetting(ctx, &store.FindInstanceSetting{Name: "BASIC"})
	require.NoError(t, err)
	currentSchemaVersion, err := ts.GetCurrentSchemaVersion()
	require.NoError(t, err)
	require.Equal(t, currentSchemaVersion, setting.GetBasicSetting().SchemaVersion)
	connection := &store.MCPConnection{
		ID: "after-upgrade", UserID: owner.ID, Label: "After upgrade", TokenHash: strings.Repeat("cd", 32),
		Status: store.MCPConnectionApproved, CreatedAt: 100, ExpiresAt: 200,
	}
	require.NoError(t, ts.CreateMCPConnection(ctx, connection))
	stored, ownerStatus, err := ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Equal(t, connection, stored)
	require.Equal(t, store.Normal, ownerStatus)
}

func TestMigrationMCPConnectionPermissionDefaultsToReadWrite(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	owner, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	connection := &store.MCPConnection{
		ID: "legacy-permission", UserID: owner.ID, Label: "Existing connector", TokenHash: strings.Repeat("d1", 32),
		Status: store.MCPConnectionApproved, CreatedAt: 100,
	}
	require.NoError(t, ts.CreateMCPConnection(ctx, connection))
	removeMCPPermissionSchema(ctx, t, ts)
	setSchemaVersion(ctx, t, ts, "26.11.9")
	require.NoError(t, ts.Migrate(ctx))
	stored, ownerStatus, err := ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Equal(t, connection, stored)
	require.Equal(t, store.MCPConnectionReadWrite, stored.Permission)
	require.Equal(t, store.Normal, ownerStatus)
}
