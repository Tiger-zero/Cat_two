package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/usememos/memos/store"
)

func (d *DB) IsLoginIPBlocked(ctx context.Context, ip string) (bool, error) {
	var blocked bool
	err := d.db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM login_ip_attempt WHERE ip = ? AND blocked_ts IS NOT NULL
	)`, ip).Scan(&blocked)
	return blocked, err
}

func (d *DB) RecordLoginIPFailure(ctx context.Context, ip string, limit int, nowTs int64) (int, bool, error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	// Insert or lock the existing row before reading the failure count.
	if _, err := tx.ExecContext(ctx, `INSERT INTO login_ip_attempt (ip, failure_count)
		VALUES (?, 0) ON DUPLICATE KEY UPDATE failure_count = failure_count`, ip); err != nil {
		return 0, false, err
	}
	var count int
	var blockedAt sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT failure_count, blocked_ts
		FROM login_ip_attempt WHERE ip = ? FOR UPDATE`, ip).Scan(&count, &blockedAt); err != nil {
		return 0, false, err
	}
	if !blockedAt.Valid {
		count++
		if count >= limit {
			blockedAt = sql.NullInt64{Int64: nowTs, Valid: true}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE login_ip_attempt SET failure_count = ?, blocked_ts = ? WHERE ip = ?`, count, blockedAt, ip); err != nil {
			return 0, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return count, blockedAt.Valid, nil
}

func (d *DB) ClearLoginIPFailures(ctx context.Context, ip string) (bool, error) {
	result, err := d.db.ExecContext(ctx, `DELETE FROM login_ip_attempt WHERE ip = ? AND blocked_ts IS NULL`, ip)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected > 0 {
		return false, err
	}
	return d.IsLoginIPBlocked(ctx, ip)
}

func (d *DB) ListBlockedLoginIPs(ctx context.Context, afterIP string, limit int) ([]*store.BlockedLoginIP, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT ip, blocked_ts, failure_count FROM login_ip_attempt
		WHERE blocked_ts IS NOT NULL AND ip > ? ORDER BY ip ASC LIMIT ?`, afterIP, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	blocks := []*store.BlockedLoginIP{}
	for rows.Next() {
		var block store.BlockedLoginIP
		var blockedTs int64
		if err := rows.Scan(&block.IP, &blockedTs, &block.FailureCount); err != nil {
			return nil, err
		}
		block.BlockedAt = time.Unix(blockedTs, 0).UTC()
		blocks = append(blocks, &block)
	}
	return blocks, rows.Err()
}

func (d *DB) UnblockLoginIP(ctx context.Context, ip string) (bool, error) {
	result, err := d.db.ExecContext(ctx, `DELETE FROM login_ip_attempt WHERE ip = ? AND blocked_ts IS NOT NULL`, ip)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}
