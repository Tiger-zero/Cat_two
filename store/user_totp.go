package store

import (
	"context"
	"database/sql"

	"github.com/pkg/errors"

	"github.com/usememos/memos/internal/random"
)

// UserTwoFactor holds the encrypted TOTP keys and the account's browser session revision.
type UserTwoFactor struct {
	ActiveSecret     string
	PendingSecret    string
	PendingExpiresTs int64
	LastUsedStep     int64
	SessionRevision  string
	FailedAttempts   int
	AttemptWindowTs  int64
}

// ErrUserTwoFactorChanged indicates that an account changed before a factor operation completed.
var ErrUserTwoFactorChanged = errors.New("user changed during two-factor authentication")

// GetUserTwoFactor returns the persisted state or the zero value for an account
// with no enrolled factor or session revision. Authentication reads bypass caches.
func (s *Store) GetUserTwoFactor(ctx context.Context, userID int32) (UserTwoFactor, error) {
	return s.driver.GetUserTwoFactor(ctx, nil, userID)
}

// UpdateUserTwoFactor updates a user's factor state atomically. expectedPasswordHash
// is nil only for an operator reset. The callback may run again after a conflict.
func (s *Store) UpdateUserTwoFactor(ctx context.Context, userID int32, expectedPasswordHash *string, update func(*UserTwoFactor) error) error {
	return s.runSerializable(ctx, func(tx *sql.Tx) error {
		users, err := s.driver.ListUsersTx(ctx, tx, &FindUser{ID: &userID})
		if err != nil {
			return err
		}
		if len(users) != 1 || users[0].RowStatus != Normal ||
			(expectedPasswordHash != nil && users[0].PasswordHash != *expectedPasswordHash) {
			return ErrUserTwoFactorChanged
		}
		before, err := s.driver.GetUserTwoFactor(ctx, tx, userID)
		if err != nil {
			return err
		}
		after := before
		if err := update(&after); err != nil {
			return err
		}
		if after == before {
			return nil
		}
		return s.driver.UpsertUserTwoFactorTx(ctx, tx, userID, after)
	})
}

// ResetUserTwoFactor removes the user's factor and invalidates all existing
// browser sessions. Personal access tokens remain independently revocable.
func (s *Store) ResetUserTwoFactor(ctx context.Context, userID int32) error {
	revision := random.UUID()
	return s.UpdateUserTwoFactor(ctx, userID, nil, func(current *UserTwoFactor) error {
		*current = UserTwoFactor{SessionRevision: revision}
		return nil
	})
}
