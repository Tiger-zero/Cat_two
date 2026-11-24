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
		updated bool
		err     error
	}
	results := make(chan change, 2)
	for range 2 {
		go func() {
			<-start
			updated, err := ts.UpdateMCPConnection(ctx, owner.ID, connection.ID, store.MCPConnectionApproved, store.MCPConnectionRevoked, 0)
			results <- change{updated: updated, err: err}
		}()
	}
	close(start)
	first, second := <-results, <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.NotEqual(t, first.updated, second.updated, "exactly one concurrent transition may succeed")
	require.NoError(t, ts.RecordMCPConnectionFirstUse(ctx, connection.TokenHash, 130, "2001:db8::1"))
	stored, ownerStatus, err = ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Equal(t, store.Archived, ownerStatus)
	require.Equal(t, store.MCPConnectionRevoked, stored.Status)
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

func TestMCPConnectionFirstUseSkipsRevokedToken(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	ts := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, ts.Close()) })
	owner, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	connection := &store.MCPConnection{
		ID: "revoked", UserID: owner.ID, TokenHash: "revoked-token-hash", Label: "Revoked connection",
		Status: store.MCPConnectionRevoked, CreatedAt: 100,
	}
	require.NoError(t, ts.CreateMCPConnection(ctx, connection))
	require.NoError(t, ts.RecordMCPConnectionFirstUse(ctx, connection.TokenHash, 110, "192.0.2.1"))
	stored, ownerStatus, err := ts.GetMCPConnectionByHash(ctx, connection.TokenHash)
	require.NoError(t, err)
	require.Equal(t, connection, stored)
	require.Equal(t, store.Normal, ownerStatus)
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
