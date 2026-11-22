package postgres

import (
	"context"
	"time"

	"github.com/usememos/memos/store"
)

func (d *DB) IsLoginIPBlocked(ctx context.Context, ip string) (bool, error) {
	var blocked bool
	err := d.db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM login_ip_attempt WHERE ip = $1 AND blocked_ts IS NOT NULL
	)`, ip).Scan(&blocked)
	return blocked, err
}

func (d *DB) RecordLoginIPFailure(ctx context.Context, ip string, limit int, nowTs int64) (int, bool, error) {
	var count int
	var blocked bool
	err := d.db.QueryRowContext(ctx, `INSERT INTO login_ip_attempt (ip, failure_count, blocked_ts)
		VALUES ($1, 1, CASE WHEN $2::integer = 1 THEN $3::bigint END)
		ON CONFLICT (ip) DO UPDATE SET
			failure_count = CASE WHEN login_ip_attempt.blocked_ts IS NULL
				THEN login_ip_attempt.failure_count + 1 ELSE login_ip_attempt.failure_count END,
			blocked_ts = COALESCE(login_ip_attempt.blocked_ts,
				CASE WHEN login_ip_attempt.failure_count + 1 >= $2 THEN $3 END)
		RETURNING failure_count, blocked_ts IS NOT NULL`, ip, limit, nowTs).Scan(&count, &blocked)
	return count, blocked, err
}

func (d *DB) ClearLoginIPFailures(ctx context.Context, ip string) (bool, error) {
	result, err := d.db.ExecContext(ctx, `DELETE FROM login_ip_attempt WHERE ip = $1 AND blocked_ts IS NULL`, ip)
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
		WHERE blocked_ts IS NOT NULL AND ip > $1 ORDER BY ip ASC LIMIT $2`, afterIP, limit)
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
	result, err := d.db.ExecContext(ctx, `DELETE FROM login_ip_attempt WHERE ip = $1 AND blocked_ts IS NOT NULL`, ip)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}
