package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/usememos/memos/store"
)

func (d *DB) GetUserTwoFactor(ctx context.Context, tx *sql.Tx, userID int32) (store.UserTwoFactor, error) {
	var executor dbExecutor = d.db
	query := `SELECT active_secret, pending_secret, pending_expires_ts, last_used_step, session_revision, failed_attempts, attempt_window_ts
FROM user_totp WHERE user_id = $1`
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
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (user_id) DO UPDATE SET active_secret = excluded.active_secret,
  pending_secret = excluded.pending_secret,
  pending_expires_ts = excluded.pending_expires_ts,
  last_used_step = excluded.last_used_step,
  session_revision = excluded.session_revision,
  failed_attempts = excluded.failed_attempts,
  attempt_window_ts = excluded.attempt_window_ts`, userID, state.ActiveSecret, state.PendingSecret, state.PendingExpiresTs, state.LastUsedStep,
		state.SessionRevision, state.FailedAttempts, state.AttemptWindowTs)
	return err
}
