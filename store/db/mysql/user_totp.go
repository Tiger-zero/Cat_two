package mysql

import (
	"context"
	"database/sql"
	"errors"

	"github.com/usememos/memos/store"
)

func (d *DB) GetUserTwoFactor(ctx context.Context, tx *sql.Tx, userID int32) (store.UserTwoFactor, error) {
	var executor dbExecutor = d.db
	query := `SELECT active_secret, pending_secret, pending_expires_ts, last_used_step, session_revision, failed_attempts, attempt_window_ts
FROM user_totp WHERE user_id = ?`
	if tx != nil {
		executor = tx
		query += " FOR UPDATE"
	}
	var state store.UserTwoFactor
	err := executor.QueryRowContext(ctx, query, userID).Scan(
		&state.ActiveSecret, &state.PendingSecret, &state.PendingExpiresTs, &state.LastUsedStep,
		&state.SessionRevision, &state.FailedAttempts, &state.AttemptWindowTs,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return store.UserTwoFactor{}, nil
	}
	return state, err
}

func (*DB) UpsertUserTwoFactorTx(ctx context.Context, tx *sql.Tx, userID int32, state store.UserTwoFactor) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO user_totp
  (user_id, active_secret, pending_secret, pending_expires_ts, last_used_step, session_revision, failed_attempts, attempt_window_ts)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE active_secret = VALUES(active_secret),
  pending_secret = VALUES(pending_secret),
  pending_expires_ts = VALUES(pending_expires_ts),
  last_used_step = VALUES(last_used_step),
  session_revision = VALUES(session_revision),
  failed_attempts = VALUES(failed_attempts),
  attempt_window_ts = VALUES(attempt_window_ts)`, userID, state.ActiveSecret, state.PendingSecret, state.PendingExpiresTs, state.LastUsedStep,
		state.SessionRevision, state.FailedAttempts, state.AttemptWindowTs)
	return err
}
