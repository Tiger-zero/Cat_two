package store

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	storepb "github.com/usememos/memos/proto/gen/store"
)

type refreshToken = storepb.RefreshTokensUserSetting_RefreshToken

const UserSessionIdleTimeout = 45 * time.Minute

// GetUserRefreshTokens returns the current refresh credential of each session.
func (s *Store) GetUserRefreshTokens(ctx context.Context, userID int32) ([]*refreshToken, error) {
	setting, err := s.getUserCredentialSetting(ctx, userID, storepb.UserSetting_REFRESH_TOKENS)
	if err != nil {
		return nil, err
	}
	return setting.GetRefreshTokens().GetRefreshTokens(), nil
}

func refreshSessionID(token *refreshToken) string {
	if id := token.GetSessionId(); id != "" {
		return id
	}
	return token.GetTokenId()
}

func sessionMatches(token *refreshToken, id string) bool {
	return id != "" && (refreshSessionID(token) == id || token.GetTokenId() == id)
}

func sessionLastEdit(token *refreshToken) *timestamppb.Timestamp {
	if last := token.GetLastEditAt(); last != nil {
		return last
	}
	return token.GetCreatedAt()
}

// UserSessionIdleExpiresAt returns the deadline for the session's next edit.
// Legacy records use the creation time of their current refresh credential.
func UserSessionIdleExpiresAt(token *refreshToken) time.Time {
	if last := sessionLastEdit(token); last != nil && last.IsValid() {
		return last.AsTime().Add(UserSessionIdleTimeout)
	}
	return time.Time{}
}

func isUnexpiredRefreshToken(token *refreshToken, now time.Time) bool {
	return token != nil && (token.ExpiresAt == nil || token.ExpiresAt.AsTime().After(now))
}

func (s *Store) isLiveRefreshToken(token *refreshToken, now time.Time) bool {
	return isUnexpiredRefreshToken(token, now) &&
		(!s.IsPersonalMode() || UserSessionIdleExpiresAt(token).After(now))
}

// IsUserSessionActive verifies that the session has an unexpired credential
// and returns its edit deadline. Read the database so other processes see
// edits and revocations immediately.
func (s *Store) IsUserSessionActive(ctx context.Context, userID int32, sessionID string) (bool, time.Time, error) {
	tokens, err := s.GetUserRefreshTokens(ctx, userID)
	if err != nil {
		return false, time.Time{}, err
	}
	now := time.Now()
	for _, token := range tokens {
		if sessionID != "" && refreshSessionID(token) == sessionID && isUnexpiredRefreshToken(token, now) {
			deadline := UserSessionIdleExpiresAt(token)
			return !s.IsPersonalMode() || deadline.After(now), deadline, nil
		}
	}
	return false, time.Time{}, nil
}

// AddUserRefreshToken records a new session and prunes expired entries.
func (s *Store) AddUserRefreshToken(ctx context.Context, userID int32, token *refreshToken) error {
	_, err := s.updateUserRefreshTokens(ctx, userID, func(tokens []*refreshToken) ([]*refreshToken, bool) {
		return append(s.pruneExpiredRefreshTokens(tokens, time.Now()), token), true
	})
	return err
}

// RotateUserRefreshToken changes the latest credential within a live session.
// A legacy record uses its original token ID and anchors inactivity to its
// current creation time before replacing the credential.
func (s *Store) RotateUserRefreshToken(ctx context.Context, userID int32, oldID string, replacement *refreshToken) (bool, time.Time, error) {
	var deadline time.Time
	changed, err := s.updateUserRefreshTokens(ctx, userID, func(tokens []*refreshToken) ([]*refreshToken, bool) {
		live := s.pruneExpiredRefreshTokens(tokens, time.Now())
		for i, token := range live {
			if token.GetTokenId() == oldID && refreshSessionID(token) == replacement.GetSessionId() {
				rotated := proto.CloneOf(replacement)
				rotated.LastEditAt = sessionLastEdit(token)
				deadline = UserSessionIdleExpiresAt(rotated)
				live[i] = rotated
				return live, true
			}
		}
		return nil, false
	})
	if err != nil || !changed {
		return false, time.Time{}, err
	}
	return true, deadline, nil
}

// RecordUserSessionEdit advances a live session's idle deadline. Recheck it
// inside the compare-and-set loop so a late edit cannot revive an expired or
// concurrently revoked session.
func (s *Store) RecordUserSessionEdit(ctx context.Context, userID int32, sessionID string) (time.Time, error) {
	var deadline time.Time
	changed, err := s.updateUserRefreshTokens(ctx, userID, func(tokens []*refreshToken) ([]*refreshToken, bool) {
		now := time.Now()
		for i, token := range tokens {
			if sessionID == "" || refreshSessionID(token) != sessionID || !s.isLiveRefreshToken(token, now) {
				continue
			}
			updated := proto.CloneOf(token)
			if last := sessionLastEdit(token); last == nil || now.After(last.AsTime()) {
				updated.LastEditAt = timestamppb.New(now)
			}
			deadline = UserSessionIdleExpiresAt(updated)
			tokens[i] = updated
			return tokens, true
		}
		return nil, false
	})
	if err != nil || !changed {
		return time.Time{}, err
	}
	return deadline, nil
}

func (s *Store) pruneExpiredRefreshTokens(tokens []*refreshToken, now time.Time) []*refreshToken {
	live := make([]*refreshToken, 0, len(tokens))
	for _, token := range tokens {
		if s.isLiveRefreshToken(token, now) {
			live = append(live, token)
		}
	}
	return live
}

// RemoveUserRefreshTokensExcept revokes all sessions except the supplied
// stable session (or a legacy refresh token ID). An empty ID revokes all.
func (s *Store) RemoveUserRefreshTokensExcept(ctx context.Context, userID int32, keepID string) error {
	_, err := s.updateUserRefreshTokens(ctx, userID, func(tokens []*refreshToken) ([]*refreshToken, bool) {
		var kept []*refreshToken
		for _, token := range tokens {
			if sessionMatches(token, keepID) {
				kept = append(kept, token)
			}
		}
		return kept, len(kept) != len(tokens)
	})
	return err
}

// RemoveUserRefreshToken revokes the session by its stable ID or current token ID.
func (s *Store) RemoveUserRefreshToken(ctx context.Context, userID int32, id string) error {
	_, err := s.updateUserRefreshTokens(ctx, userID, func(tokens []*refreshToken) ([]*refreshToken, bool) {
		kept := make([]*refreshToken, 0, len(tokens))
		for _, token := range tokens {
			if !sessionMatches(token, id) {
				kept = append(kept, token)
			}
		}
		return kept, len(kept) != len(tokens)
	})
	return err
}

// GetUserRefreshTokenByID returns the specific current refresh credential.
func (s *Store) GetUserRefreshTokenByID(ctx context.Context, userID int32, tokenID string) (*refreshToken, error) {
	tokens, err := s.GetUserRefreshTokens(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, token := range tokens {
		if token.GetTokenId() == tokenID {
			return token, nil
		}
	}
	return nil, nil
}

// updateUserRefreshTokens retries if another database connection committed a
// session mutation first, so a rotation cannot restore a revoked credential.
func (s *Store) updateUserRefreshTokens(ctx context.Context, userID int32, update func([]*refreshToken) ([]*refreshToken, bool)) (bool, error) {
	s.refreshTokenMu.Lock()
	defer s.refreshTokenMu.Unlock()

	const key = storepb.UserSetting_REFRESH_TOKENS
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		records, err := s.driver.ListUserSettings(ctx, &FindUserSetting{UserID: &userID, Key: key})
		if err != nil {
			return false, err
		}
		var previous *string
		var tokens []*refreshToken
		if len(records) != 0 {
			previous = &records[0].Value
			setting := &storepb.RefreshTokensUserSetting{}
			if err := protojsonUnmarshaler.Unmarshal([]byte(*previous), setting); err != nil {
				return false, errors.Wrap(err, "decode refresh token setting")
			}
			tokens = setting.GetRefreshTokens()
		}
		next, changed := update(tokens)
		if !changed {
			return false, nil
		}
		value, err := protojson.Marshal(&storepb.RefreshTokensUserSetting{RefreshTokens: next})
		if err != nil {
			return false, errors.Wrap(err, "encode refresh token setting")
		}
		written, err := s.driver.CompareAndSetUserSetting(ctx, &UserSetting{UserID: userID, Key: key, Value: string(value)}, previous)
		if err != nil {
			return false, err
		}
		if written {
			s.userSettingCache.Delete(ctx, getUserSettingCacheKey(userID, key.String()))
			return true, nil
		}
	}
}
