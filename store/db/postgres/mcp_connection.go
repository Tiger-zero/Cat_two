package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/usememos/memos/store"
)

const mcpConnectionColumns = "id, user_id, label, token_hash, status, created_ts, requested_ts, expires_ts, remote_ip, permission"

func (d *DB) CreateMCPConnection(ctx context.Context, connection *store.MCPConnection) error {
	_, err := d.db.ExecContext(ctx, `INSERT INTO mcp_connection (`+mcpConnectionColumns+`)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`, connection.ID, connection.UserID, connection.Label, connection.TokenHash, connection.Status,
		connection.CreatedAt, connection.RequestedAt, connection.ExpiresAt, connection.RemoteIP, connection.Permission,
	)
	return err
}

func (d *DB) ListMCPConnections(ctx context.Context, userID int32) ([]*store.MCPConnection, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT `+mcpConnectionColumns+`
FROM mcp_connection WHERE user_id = $1 ORDER BY created_ts DESC, id DESC`, userID)
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

func (d *DB) GetMCPConnectionByHash(ctx context.Context, tokenHash string) (*store.MCPConnection, store.RowStatus, error) {
	var connection store.MCPConnection
	var ownerStatus store.RowStatus
	err := d.db.QueryRowContext(ctx, `
		SELECT c.id, c.user_id, c.label, c.token_hash, c.status,
		       c.created_ts, c.requested_ts, c.expires_ts, c.remote_ip, c.permission, u.row_status
		FROM mcp_connection c
		JOIN "user" u ON u.id = c.user_id
		WHERE c.token_hash = $1`, tokenHash).Scan(
		&connection.ID, &connection.UserID, &connection.Label, &connection.TokenHash, &connection.Status,
		&connection.CreatedAt, &connection.RequestedAt, &connection.ExpiresAt, &connection.RemoteIP, &connection.Permission, &ownerStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return &connection, ownerStatus, nil
}

func (d *DB) RecordMCPConnectionFirstUse(ctx context.Context, tokenHash string, requestedAt int64, remoteIP string) error {
	_, err := d.db.ExecContext(ctx, `UPDATE mcp_connection SET requested_ts = $1, remote_ip = $2
WHERE token_hash = $3 AND status = $4 AND requested_ts = 0`,
		requestedAt, remoteIP, tokenHash, store.MCPConnectionApproved)
	return err
}

func (d *DB) UpdateMCPConnection(ctx context.Context, userID int32, id string, from store.MCPConnectionStatus, to store.MCPConnectionStatus, expiresAt int64) (bool, error) {
	result, err := d.db.ExecContext(ctx, `UPDATE mcp_connection SET status = $1, expires_ts = $2
WHERE user_id = $3 AND id = $4 AND status = $5 AND (status <> $6 OR expires_ts <> $7)`,
		to, expiresAt, userID, id, from, to, expiresAt)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (d *DB) DeleteMCPConnection(ctx context.Context, userID int32, id string) (bool, error) {
	result, err := d.db.ExecContext(ctx, `DELETE FROM mcp_connection WHERE user_id = $1 AND id = $2`, userID, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func scanMCPConnection(row interface{ Scan(...any) error }) (*store.MCPConnection, error) {
	connection := &store.MCPConnection{}
	if err := row.Scan(&connection.ID, &connection.UserID, &connection.Label, &connection.TokenHash, &connection.Status,
		&connection.CreatedAt, &connection.RequestedAt, &connection.ExpiresAt, &connection.RemoteIP, &connection.Permission); err != nil {
		return nil, err
	}
	return connection, nil
}
