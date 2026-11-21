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
		Status: store.MCPConnectionAwaiting, CreatedAt: 100, ExpiresAt: 200,
	}
	require.NoError(t, ts.CreateMCPConnection(ctx, connection))
	duplicate := *connection
	duplicate.ID, duplicate.UserID = "duplicate-hash", other.ID
	require.Error(t, ts.CreateMCPConnection(ctx, &duplicate))
	foreign := &store.MCPConnection{
		ID: "foreign", UserID: other.ID, Label: "Other", TokenHash: strings.Repeat("bc", 32),
		Status: store.MCPConnectionAwaiting, CreatedAt: 101, ExpiresAt: 200,
	}
	require.NoError(t, ts.CreateMCPConnection(ctx, foreign))

	list, err := ts.ListMCPConnections(ctx, owner.ID)
	require.NoError(t, err)
	require.Equal(t, []*store.MCPConnection{connection}, list)
	encoded, err := json.Marshal(list[0])
	require.NoError(t, err)
	require.NotContains(t, string(encoded), connection.TokenHash)
	require.NotContains(t, string(encoded), "userId")
	stored, err := ts.GetMCPConnectionByHash(ctx, strings.ToUpper(connection.TokenHash))
	require.NoError(t, err)
	require.Nil(t, stored)

	require.NoError(t, ts.MarkMCPConnectionRequested(ctx, connection.TokenHash, 110, "192.0.2.1"))
	require.NoError(t, ts.MarkMCPConnectionRequested(ctx, connection.TokenHash, 120, "192.0.2.2"))
	stored, err = ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Equal(t, store.MCPConnectionPending, stored.Status)
	require.EqualValues(t, 110, stored.RequestedAt)
	require.Equal(t, "192.0.2.1", stored.RemoteIP)
	require.EqualValues(t, 200, stored.ExpiresAt)
	updated, err := ts.UpdateMCPConnection(ctx, other.ID, connection.ID, store.MCPConnectionPending, store.MCPConnectionApproved, 300)
	require.NoError(t, err)
	require.False(t, updated)

	start := make(chan struct{})
	type change struct {
		status  store.MCPConnectionStatus
		expires int64
		updated bool
		err     error
	}
	results := make(chan change, 2)
	for _, status := range []store.MCPConnectionStatus{store.MCPConnectionApproved, store.MCPConnectionDenied} {
		go func() {
			<-start
			expires := int64(0)
			if status == store.MCPConnectionApproved {
				expires = 300
			}
			updated, err := ts.UpdateMCPConnection(ctx, owner.ID, connection.ID, store.MCPConnectionPending, status, expires)
			results <- change{status: status, expires: expires, updated: updated, err: err}
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
	require.NoError(t, ts.MarkMCPConnectionRequested(ctx, connection.TokenHash, 130, "2001:db8::1"))
	stored, err = ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Equal(t, winner.status, stored.Status)
	require.Equal(t, winner.expires, stored.ExpiresAt)
	require.EqualValues(t, 110, stored.RequestedAt)
	require.Equal(t, "192.0.2.1", stored.RemoteIP)

	_, err = ts.DeleteUser(ctx, &store.DeleteUser{ID: owner.ID})
	require.NoError(t, err)
	stored, err = ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Nil(t, stored)
	require.Error(t, ts.CreateMCPConnection(ctx, connection), "a deleted account cannot own another connection")
	stored, err = ts.GetMCPConnectionByHash(ctx, foreign.TokenHash)
	require.NoError(t, err)
	require.Equal(t, foreign, stored)
}

func TestMigrationMCPConnection(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	owner, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	_, err = ts.GetDriver().GetDB().ExecContext(ctx, "DROP TABLE mcp_connection")
	require.NoError(t, err)
	setSchemaVersion(ctx, t, ts, "26.11.3")
	require.NoError(t, ts.Migrate(ctx))
	require.NoError(t, ts.Migrate(ctx), "a completed migration can restart")
	setting, err := ts.GetUncachedInstanceSetting(ctx, &store.FindInstanceSetting{Name: "BASIC"})
	require.NoError(t, err)
	require.Equal(t, "26.11.4", setting.GetBasicSetting().SchemaVersion)
	connection := &store.MCPConnection{
		ID: "after-upgrade", UserID: owner.ID, Label: "After upgrade", TokenHash: strings.Repeat("cd", 32),
		Status: store.MCPConnectionAwaiting, CreatedAt: 100,
	}
	require.NoError(t, ts.CreateMCPConnection(ctx, connection))
	stored, err := ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Equal(t, connection, stored)
}
