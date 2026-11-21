package mysql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/usememos/memos/store"
)

const mcpConnectionColumns = "id, user_id, label, token_hash, status, created_ts, requested_ts, expires_ts, remote_ip"

func (d *DB) CreateMCPConnection(ctx context.Context, connection *store.MCPConnection) error {
	_, err := d.db.ExecContext(ctx, `INSERT INTO mcp_connection (`+mcpConnectionColumns+`)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, connection.ID, connection.UserID, connection.Label, connection.TokenHash, connection.Status,
		connection.CreatedAt, connection.RequestedAt, connection.ExpiresAt, connection.RemoteIP,
	)
	return err
}

func (d *DB) ListMCPConnections(ctx context.Context, userID int32) ([]*store.MCPConnection, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+mcpConnectionColumns+`
FROM mcp_connection WHERE user_id = ? ORDER BY created_ts DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	connections := []*store.MCPConnection{}
	for rows.Next() {
		connection, err := scanMCPConnection(rows)
		if err != nil {
			return nil, err
		}
		connections = append(connections, connection)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return connections, nil
}

func (d *DB) GetMCPConnectionByHash(ctx context.Context, tokenHash string) (*store.MCPConnection, error) {
	connection, err := scanMCPConnection(d.db.QueryRowContext(ctx,
		`SELECT `+mcpConnectionColumns+` FROM mcp_connection WHERE token_hash = ?`, tokenHash))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return connection, err
}

func (d *DB) RecordMCPConnectionFirstUse(ctx context.Context, tokenHash string, requestedAt int64, remoteIP string) error {
	_, err := d.db.ExecContext(ctx, `UPDATE mcp_connection SET requested_ts = ?, remote_ip = ?
WHERE token_hash = ? AND status = ? AND requested_ts = 0`,
		requestedAt, remoteIP, tokenHash, store.MCPConnectionApproved)
	return err
}

func (d *DB) UpdateMCPConnection(ctx context.Context, userID int32, id string, from store.MCPConnectionStatus, to store.MCPConnectionStatus, expiresAt int64) (bool, error) {
	result, err := d.db.ExecContext(ctx, `UPDATE mcp_connection SET status = ?, expires_ts = ?
WHERE user_id = ? AND id = ? AND status = ? AND (status <> ? OR expires_ts <> ?)`,
		to, expiresAt, userID, id, from, to, expiresAt)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func scanMCPConnection(row interface{ Scan(...any) error }) (*store.MCPConnection, error) {
	connection := &store.MCPConnection{}
	if err := row.Scan(&connection.ID, &connection.UserID, &connection.Label, &connection.TokenHash, &connection.Status,
		&connection.CreatedAt, &connection.RequestedAt, &connection.ExpiresAt, &connection.RemoteIP); err != nil {
		return nil, err
	}
	return connection, nil
}
