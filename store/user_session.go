package store

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	storepb "github.com/usememos/memos/proto/gen/store"
)

type refreshToken = storepb.RefreshTokensUserSetting_RefreshToken

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

func isLiveRefreshToken(token *refreshToken, now time.Time) bool {
	return token != nil && (token.ExpiresAt == nil || token.ExpiresAt.AsTime().After(now))
}

// IsUserSessionActive verifies that the session has an unexpired credential.
// Read this from the database so other processes see revocations immediately.
func (s *Store) IsUserSessionActive(ctx context.Context, userID int32, sessionID string) (bool, error) {
	tokens, err := s.GetUserRefreshTokens(ctx, userID)
	if err != nil {
		return false, err
	}
	now := time.Now()
	for _, token := range tokens {
		if refreshSessionID(token) == sessionID && isLiveRefreshToken(token, now) {
			return true, nil
		}
	}
	return false, nil
}

// AddUserRefreshToken records a new session and prunes expired entries.
func (s *Store) AddUserRefreshToken(ctx context.Context, userID int32, token *refreshToken) error {
	_, err := s.updateUserRefreshTokens(ctx, userID, func(tokens []*refreshToken) ([]*refreshToken, bool) {
		return append(pruneExpiredRefreshTokens(tokens, time.Now()), token), true
	})
	return err
}

// RotateUserRefreshToken changes the latest credential within a live session.
// A legacy record upgrades to a session whose ID is its original token ID.
func (s *Store) RotateUserRefreshToken(ctx context.Context, userID int32, oldID string, replacement *refreshToken) (bool, error) {
	return s.updateUserRefreshTokens(ctx, userID, func(tokens []*refreshToken) ([]*refreshToken, bool) {
		live := pruneExpiredRefreshTokens(tokens, time.Now())
		for i, token := range live {
			if token.GetTokenId() == oldID && refreshSessionID(token) == replacement.GetSessionId() {
				live[i] = replacement
				return live, true
			}
		}
		return nil, false
	})
}

func pruneExpiredRefreshTokens(tokens []*refreshToken, now time.Time) []*refreshToken {
	live := make([]*refreshToken, 0, len(tokens))
	for _, token := range tokens {
		if isLiveRefreshToken(token, now) {
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
