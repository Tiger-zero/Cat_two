package store

import "context"

// MCPConnectionStatus is the stored state of an MCP connection.
type MCPConnectionStatus string

const (
	MCPConnectionAwaiting MCPConnectionStatus = "awaiting"
	MCPConnectionPending  MCPConnectionStatus = "pending"
	MCPConnectionApproved MCPConnectionStatus = "approved"
	MCPConnectionDenied   MCPConnectionStatus = "denied"
	MCPConnectionRevoked  MCPConnectionStatus = "revoked"
)

// MCPConnection stores authorization state and the token hash for a client.
// Timestamps are Unix seconds. A zero ExpiresAt means there is no expiration.
type MCPConnection struct {
	ID          string              `json:"id"`
	UserID      int32               `json:"-"`
	Label       string              `json:"label"`
	TokenHash   string              `json:"-"`
	Status      MCPConnectionStatus `json:"status"`
	CreatedAt   int64               `json:"createdAt"`
	RequestedAt int64               `json:"requestedAt"`
	ExpiresAt   int64               `json:"expiresAt"`
	RemoteIP    string              `json:"remoteIp"`
}

// CreateMCPConnection persists a connection without changing a previous record.
func (s *Store) CreateMCPConnection(ctx context.Context, connection *MCPConnection) error {
	return s.driver.CreateMCPConnection(ctx, connection)
}

// ListMCPConnections returns one user's connections, newest first.
func (s *Store) ListMCPConnections(ctx context.Context, userID int32) ([]*MCPConnection, error) {
	return s.driver.ListMCPConnections(ctx, userID)
}

// GetMCPConnectionByHash returns the connection for a token hash, or nil if absent.
func (s *Store) GetMCPConnectionByHash(ctx context.Context, tokenHash string) (*MCPConnection, error) {
	return s.driver.GetMCPConnectionByHash(ctx, tokenHash)
}

// RecordMCPConnectionFirstUse records the first request's time and IP for an
// approved connection. Later requests preserve those values.
func (s *Store) RecordMCPConnectionFirstUse(ctx context.Context, tokenHash string, requestedAt int64, remoteIP string) error {
	return s.driver.RecordMCPConnectionFirstUse(ctx, tokenHash, requestedAt, remoteIP)
}

// UpdateMCPConnection changes the status and expiry only when the specified
// connection belongs to the user and its state matches from. It reports whether
// the row changed.
func (s *Store) UpdateMCPConnection(ctx context.Context, userID int32, id string, from MCPConnectionStatus, to MCPConnectionStatus, expiresAt int64) (bool, error) {
	return s.driver.UpdateMCPConnection(ctx, userID, id, from, to, expiresAt)
}
