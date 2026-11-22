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

type UserSetting struct {
	UserID int32
	Key    storepb.UserSetting_Key
	Value  string
}

type FindUserSetting struct {
	UserID *int32
	Key    storepb.UserSetting_Key
}

type DeleteUserSetting struct {
	UserID *int32
	Key    storepb.UserSetting_Key
}

// RefreshTokenQueryResult contains the result of querying a refresh token.
type RefreshTokenQueryResult struct {
	UserID       int32
	RefreshToken *storepb.RefreshTokensUserSetting_RefreshToken
}

// PATQueryResult contains the result of querying a PAT by hash.
type PATQueryResult struct {
	UserID int32
	User   *User
	PAT    *storepb.PersonalAccessTokensUserSetting_PersonalAccessToken
}

func (s *Store) UpsertUserSetting(ctx context.Context, upsert *storepb.UserSetting) (*storepb.UserSetting, error) {
	userSettingRaw, err := convertUserSettingToRaw(upsert)
	if err != nil {
		return nil, err
	}
	userSettingRaw, err = s.driver.UpsertUserSetting(ctx, userSettingRaw)
	if err != nil {
		return nil, err
	}

	userSetting, err := convertUserSettingFromRaw(userSettingRaw)
	if err != nil {
		return nil, err
	}
	if userSetting == nil {
		return nil, errors.New("unexpected nil user setting")
	}
	s.userSettingCache.Set(ctx, getUserSettingCacheKey(userSetting.UserId, userSetting.Key.String()), userSetting)
	return userSetting, nil
}

func (s *Store) ListUserSettings(ctx context.Context, find *FindUserSetting) ([]*storepb.UserSetting, error) {
	userSettingRawList, err := s.driver.ListUserSettings(ctx, find)
	if err != nil {
		return nil, err
	}

	userSettings := []*storepb.UserSetting{}
	for _, userSettingRaw := range userSettingRawList {
		userSetting, err := convertUserSettingFromRaw(userSettingRaw)
		if err != nil {
			return nil, err
		}
		if userSetting == nil {
			continue
		}
		s.userSettingCache.Set(ctx, getUserSettingCacheKey(userSetting.UserId, userSetting.Key.String()), userSetting)
		userSettings = append(userSettings, userSetting)
	}
	return userSettings, nil
}

func (s *Store) GetUserSetting(ctx context.Context, find *FindUserSetting) (*storepb.UserSetting, error) {
	if find.UserID != nil {
		if cache, ok := s.userSettingCache.Get(ctx, getUserSettingCacheKey(*find.UserID, find.Key.String())); ok {
			userSetting, ok := cache.(*storepb.UserSetting)
			if ok {
				return userSetting, nil
			}
		}
	}

	list, err := s.ListUserSettings(ctx, find)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	if len(list) > 1 {
		return nil, errors.Errorf("expected 1 user setting, but got %d", len(list))
	}

	userSetting := list[0]
	s.userSettingCache.Set(ctx, getUserSettingCacheKey(userSetting.UserId, userSetting.Key.String()), userSetting)
	return userSetting, nil
}

func (s *Store) DeleteUserSettings(ctx context.Context, delete *DeleteUserSetting) error {
	existing, err := s.ListUserSettings(ctx, &FindUserSetting{
		UserID: delete.UserID,
		Key:    delete.Key,
	})
	if err != nil {
		return err
	}
	if err := s.driver.DeleteUserSettings(ctx, delete); err != nil {
		return err
	}
	for _, setting := range existing {
		s.userSettingCache.Delete(ctx, getUserSettingCacheKey(setting.UserId, setting.Key.String()))
	}
	return nil
}

// GetUserByPATHash finds the user owning the personal access token with the
// given hash. It returns (nil, nil) when no token matches or its user no longer
// exists, and an error only when the lookup itself fails.
func (s *Store) GetUserByPATHash(ctx context.Context, tokenHash string) (*PATQueryResult, error) {
	result, err := s.driver.GetUserByPATHash(ctx, tokenHash)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}

	user, err := s.GetUser(ctx, &FindUser{ID: &result.UserID})
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}
	result.User = user

	return result, nil
}

// GetUserRefreshTokens returns the refresh tokens of the user.
func (s *Store) GetUserRefreshTokens(ctx context.Context, userID int32) ([]*storepb.RefreshTokensUserSetting_RefreshToken, error) {
	setting, err := s.getUserCredentialSetting(ctx, userID, storepb.UserSetting_REFRESH_TOKENS)
	if err != nil {
		return nil, err
	}
	if setting == nil {
		return []*storepb.RefreshTokensUserSetting_RefreshToken{}, nil
	}
	return setting.GetRefreshTokens().GetRefreshTokens(), nil
}

// AddUserRefreshToken adds a new refresh token for the user.
func (s *Store) AddUserRefreshToken(ctx context.Context, userID int32, token *storepb.RefreshTokensUserSetting_RefreshToken) error {
	s.refreshTokenMu.Lock()
	defer s.refreshTokenMu.Unlock()

	tokens, err := s.GetUserRefreshTokens(ctx, userID)
	if err != nil {
		return err
	}

	// Expired tokens can never authenticate again; drop them here so the
	// list is bounded by the tokens issued within one lifetime rather than
	// by every sign-in the account ever made.
	tokens = append(pruneExpiredRefreshTokens(tokens, time.Now()), token)
	return s.setUserRefreshTokens(ctx, userID, tokens)
}

// RotateUserRefreshToken replaces a still-live token in one settings write.
// It returns false when the old token has already been used or revoked.
func (s *Store) RotateUserRefreshToken(ctx context.Context, userID int32, oldTokenID string, replacement *storepb.RefreshTokensUserSetting_RefreshToken) (bool, error) {
	s.refreshTokenMu.Lock()
	defer s.refreshTokenMu.Unlock()

	tokens, err := s.GetUserRefreshTokens(ctx, userID)
	if err != nil {
		return false, err
	}
	live := pruneExpiredRefreshTokens(tokens, time.Now())
	for i, token := range live {
		if token.GetTokenId() == oldTokenID {
			live[i] = replacement
			if err := s.setUserRefreshTokens(ctx, userID, live); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) setUserRefreshTokens(ctx context.Context, userID int32, tokens []*storepb.RefreshTokensUserSetting_RefreshToken) error {
	_, err := s.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: userID,
		Key:    storepb.UserSetting_REFRESH_TOKENS,
		Value: &storepb.UserSetting_RefreshTokens{
			RefreshTokens: &storepb.RefreshTokensUserSetting{
				RefreshTokens: tokens,
			},
		},
	})
	return err
}

func pruneExpiredRefreshTokens(tokens []*storepb.RefreshTokensUserSetting_RefreshToken, now time.Time) []*storepb.RefreshTokensUserSetting_RefreshToken {
	live := make([]*storepb.RefreshTokensUserSetting_RefreshToken, 0, len(tokens))
	for _, token := range tokens {
		if token == nil || (token.ExpiresAt != nil && token.ExpiresAt.AsTime().Before(now)) {
			continue
		}
		live = append(live, token)
	}
	return live
}

// RemoveUserRefreshTokensExcept revokes every refresh token of the user other
// than keepTokenID. An empty keepTokenID revokes all of them.
func (s *Store) RemoveUserRefreshTokensExcept(ctx context.Context, userID int32, keepTokenID string) error {
	s.refreshTokenMu.Lock()
	defer s.refreshTokenMu.Unlock()

	existingTokens, err := s.GetUserRefreshTokens(ctx, userID)
	if err != nil {
		return err
	}
	kept := make([]*storepb.RefreshTokensUserSetting_RefreshToken, 0, 1)
	for _, token := range existingTokens {
		if keepTokenID != "" && token.TokenId == keepTokenID {
			kept = append(kept, token)
		}
	}
	return s.setUserRefreshTokens(ctx, userID, kept)
}

// RemoveUserRefreshToken removes a refresh token from the user.
func (s *Store) RemoveUserRefreshToken(ctx context.Context, userID int32, tokenID string) error {
	s.refreshTokenMu.Lock()
	defer s.refreshTokenMu.Unlock()

	existingTokens, err := s.GetUserRefreshTokens(ctx, userID)
	if err != nil {
		return err
	}

	newTokens := make([]*storepb.RefreshTokensUserSetting_RefreshToken, 0, len(existingTokens))
	for _, token := range existingTokens {
		if token.TokenId != tokenID {
			newTokens = append(newTokens, token)
		}
	}

	return s.setUserRefreshTokens(ctx, userID, newTokens)
}

// GetUserRefreshTokenByID returns a specific refresh token.
func (s *Store) GetUserRefreshTokenByID(ctx context.Context, userID int32, tokenID string) (*storepb.RefreshTokensUserSetting_RefreshToken, error) {
	tokens, err := s.GetUserRefreshTokens(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, token := range tokens {
		if token.TokenId == tokenID {
			return token, nil
		}
	}
	return nil, nil
}

// GetUserPersonalAccessTokens returns the PATs of the user.
func (s *Store) GetUserPersonalAccessTokens(ctx context.Context, userID int32) ([]*storepb.PersonalAccessTokensUserSetting_PersonalAccessToken, error) {
	userSetting, err := s.getUserCredentialSetting(ctx, userID, storepb.UserSetting_PERSONAL_ACCESS_TOKENS)
	if err != nil {
		return nil, err
	}
	if userSetting == nil {
		return []*storepb.PersonalAccessTokensUserSetting_PersonalAccessToken{}, nil
	}
	return userSetting.GetPersonalAccessTokens().GetTokens(), nil
}

// Credential reads and mutations use the database row so that a cached snapshot
// cannot hide a revocation or restore a credential that has already been revoked.
func (s *Store) getUserCredentialSetting(ctx context.Context, userID int32, key storepb.UserSetting_Key) (*storepb.UserSetting, error) {
	settings, err := s.driver.ListUserSettings(ctx, &FindUserSetting{UserID: &userID, Key: key})
	if err != nil || len(settings) == 0 {
		return nil, err
	}
	if len(settings) > 1 {
		return nil, errors.Errorf("expected 1 user setting, but got %d", len(settings))
	}
	return convertUserSettingFromRaw(settings[0])
}

// AddUserPersonalAccessToken adds a new PAT for the user.
func (s *Store) AddUserPersonalAccessToken(ctx context.Context, userID int32, token *storepb.PersonalAccessTokensUserSetting_PersonalAccessToken) error {
	s.patMu.Lock()
	defer s.patMu.Unlock()

	tokens, err := s.GetUserPersonalAccessTokens(ctx, userID)
	if err != nil {
		return err
	}

	tokens = append(tokens, token)

	_, err = s.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: userID,
		Key:    storepb.UserSetting_PERSONAL_ACCESS_TOKENS,
		Value: &storepb.UserSetting_PersonalAccessTokens{
			PersonalAccessTokens: &storepb.PersonalAccessTokensUserSetting{
				Tokens: tokens,
			},
		},
	})
	return err
}

// RemoveUserPersonalAccessToken removes a PAT from the user.
func (s *Store) RemoveUserPersonalAccessToken(ctx context.Context, userID int32, tokenID string) error {
	s.patMu.Lock()
	defer s.patMu.Unlock()

	existingTokens, err := s.GetUserPersonalAccessTokens(ctx, userID)
	if err != nil {
		return err
	}

	newTokens := make([]*storepb.PersonalAccessTokensUserSetting_PersonalAccessToken, 0, len(existingTokens))
	for _, token := range existingTokens {
		if token.TokenId != tokenID {
			newTokens = append(newTokens, token)
		}
	}

	_, err = s.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: userID,
		Key:    storepb.UserSetting_PERSONAL_ACCESS_TOKENS,
		Value: &storepb.UserSetting_PersonalAccessTokens{
			PersonalAccessTokens: &storepb.PersonalAccessTokensUserSetting{
				Tokens: newTokens,
			},
		},
	})
	return err
}

// UpdatePATLastUsed updates the last_used_at timestamp of a PAT.
func (s *Store) UpdatePATLastUsed(ctx context.Context, userID int32, tokenID string, lastUsed *timestamppb.Timestamp) error {
	s.patMu.Lock()
	defer s.patMu.Unlock()

	tokens, err := s.GetUserPersonalAccessTokens(ctx, userID)
	if err != nil {
		return err
	}

	for i, token := range tokens {
		if token.TokenId == tokenID {
			// Concurrent requests can finish out of order. Never let an older usage
			// timestamp overwrite a newer one.
			if lastUsed != nil && token.LastUsedAt != nil && !token.LastUsedAt.AsTime().Before(lastUsed.AsTime()) {
				return nil
			}

			updatedToken, ok := proto.Clone(token).(*storepb.PersonalAccessTokensUserSetting_PersonalAccessToken)
			if !ok {
				return errors.Errorf("failed to clone personal access token")
			}
			updatedToken.LastUsedAt = lastUsed
			updatedTokens := make([]*storepb.PersonalAccessTokensUserSetting_PersonalAccessToken, len(tokens))
			copy(updatedTokens, tokens)
			updatedTokens[i] = updatedToken

			_, err = s.UpsertUserSetting(ctx, &storepb.UserSetting{
				UserId: userID,
				Key:    storepb.UserSetting_PERSONAL_ACCESS_TOKENS,
				Value: &storepb.UserSetting_PersonalAccessTokens{
					PersonalAccessTokens: &storepb.PersonalAccessTokensUserSetting{
						Tokens: updatedTokens,
					},
				},
			})
			return err
		}
	}

	return nil
}

// GetUserWebhooks returns the webhooks of the user.
func (s *Store) GetUserWebhooks(ctx context.Context, userID int32) ([]*storepb.WebhooksUserSetting_Webhook, error) {
	userSetting, err := s.GetUserSetting(ctx, &FindUserSetting{
		UserID: &userID,
		Key:    storepb.UserSetting_WEBHOOKS,
	})
	if err != nil {
		return nil, err
	}
	if userSetting == nil {
		return []*storepb.WebhooksUserSetting_Webhook{}, nil
	}

	webhooksUserSetting := userSetting.GetWebhooks()
	return webhooksUserSetting.Webhooks, nil
}

// AddUserWebhook adds a new webhook for the user.
func (s *Store) AddUserWebhook(ctx context.Context, userID int32, webhook *storepb.WebhooksUserSetting_Webhook) error {
	existingWebhooks, err := s.GetUserWebhooks(ctx, userID)
	if err != nil {
		return err
	}

	// Check if webhook already exists, update if it does
	var updatedWebhooks []*storepb.WebhooksUserSetting_Webhook
	webhookExists := false
	for _, existing := range existingWebhooks {
		if existing.Id == webhook.Id {
			updatedWebhooks = append(updatedWebhooks, webhook)
			webhookExists = true
		} else {
			updatedWebhooks = append(updatedWebhooks, existing)
		}
	}

	// If webhook doesn't exist, add it
	if !webhookExists {
		updatedWebhooks = append(updatedWebhooks, webhook)
	}

	_, err = s.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: userID,
		Key:    storepb.UserSetting_WEBHOOKS,
		Value: &storepb.UserSetting_Webhooks{
			Webhooks: &storepb.WebhooksUserSetting{
				Webhooks: updatedWebhooks,
			},
		},
	})

	return err
}

// RemoveUserWebhook removes the webhook of the user.
func (s *Store) RemoveUserWebhook(ctx context.Context, userID int32, webhookID string) error {
	oldWebhooks, err := s.GetUserWebhooks(ctx, userID)
	if err != nil {
		return err
	}

	newWebhooks := make([]*storepb.WebhooksUserSetting_Webhook, 0, len(oldWebhooks))
	for _, webhook := range oldWebhooks {
		if webhookID != webhook.Id {
			newWebhooks = append(newWebhooks, webhook)
		}
	}

	_, err = s.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: userID,
		Key:    storepb.UserSetting_WEBHOOKS,
		Value: &storepb.UserSetting_Webhooks{
			Webhooks: &storepb.WebhooksUserSetting{
				Webhooks: newWebhooks,
			},
		},
	})

	return err
}

// UpdateUserWebhook updates an existing webhook for the user.
func (s *Store) UpdateUserWebhook(ctx context.Context, userID int32, webhook *storepb.WebhooksUserSetting_Webhook) error {
	webhooks, err := s.GetUserWebhooks(ctx, userID)
	if err != nil {
		return err
	}

	for i, existing := range webhooks {
		if existing.Id == webhook.Id {
			webhooks[i] = webhook
			break
		}
	}

	_, err = s.UpsertUserSetting(ctx, &storepb.UserSetting{
		UserId: userID,
		Key:    storepb.UserSetting_WEBHOOKS,
		Value: &storepb.UserSetting_Webhooks{
			Webhooks: &storepb.WebhooksUserSetting{
				Webhooks: webhooks,
			},
		},
	})

	return err
}

func convertUserSettingFromRaw(raw *UserSetting) (*storepb.UserSetting, error) {
	userSetting := &storepb.UserSetting{
		UserId: raw.UserID,
		Key:    raw.Key,
	}

	switch raw.Key {
	case storepb.UserSetting_GENERAL:
		generalUserSetting := &storepb.GeneralUserSetting{}
		if err := protojsonUnmarshaler.Unmarshal([]byte(raw.Value), generalUserSetting); err != nil {
			return nil, err
		}
		userSetting.Value = &storepb.UserSetting_General{General: generalUserSetting}
	case storepb.UserSetting_TAGS:
		tagsUserSetting := &storepb.TagsUserSetting{}
		if err := protojsonUnmarshaler.Unmarshal([]byte(raw.Value), tagsUserSetting); err != nil {
			return nil, errors.Wrap(err, "unmarshal tags user setting")
		}
		userSetting.Value = &storepb.UserSetting_Tags{Tags: tagsUserSetting}
	case storepb.UserSetting_EDITOR:
		editorUserSetting := &storepb.EditorUserSetting{}
		if err := protojsonUnmarshaler.Unmarshal([]byte(raw.Value), editorUserSetting); err != nil {
			return nil, errors.Wrap(err, "unmarshal editor user setting")
		}
		userSetting.Value = &storepb.UserSetting_Editor{Editor: editorUserSetting}
	case storepb.UserSetting_REFRESH_TOKENS:
		refreshTokensUserSetting := &storepb.RefreshTokensUserSetting{}
		if err := protojsonUnmarshaler.Unmarshal([]byte(raw.Value), refreshTokensUserSetting); err != nil {
			return nil, err
		}
		userSetting.Value = &storepb.UserSetting_RefreshTokens{RefreshTokens: refreshTokensUserSetting}
	case storepb.UserSetting_PERSONAL_ACCESS_TOKENS:
		patsUserSetting := &storepb.PersonalAccessTokensUserSetting{}
		if err := protojsonUnmarshaler.Unmarshal([]byte(raw.Value), patsUserSetting); err != nil {
			return nil, err
		}
		userSetting.Value = &storepb.UserSetting_PersonalAccessTokens{PersonalAccessTokens: patsUserSetting}
	case storepb.UserSetting_WEBHOOKS:
		webhooksUserSetting := &storepb.WebhooksUserSetting{}
		if err := protojsonUnmarshaler.Unmarshal([]byte(raw.Value), webhooksUserSetting); err != nil {
			return nil, err
		}
		userSetting.Value = &storepb.UserSetting_Webhooks{Webhooks: webhooksUserSetting}
	default:
		// Ignore settings left in the database by older versions.
		return nil, nil
	}
	return userSetting, nil
}

func convertUserSettingToRaw(userSetting *storepb.UserSetting) (*UserSetting, error) {
	raw := &UserSetting{
		UserID: userSetting.UserId,
		Key:    userSetting.Key,
	}

	switch userSetting.Key {
	case storepb.UserSetting_GENERAL:
		generalUserSetting := userSetting.GetGeneral()
		value, err := protojson.Marshal(generalUserSetting)
		if err != nil {
			return nil, err
		}
		raw.Value = string(value)
	case storepb.UserSetting_TAGS:
		tagsUserSetting := userSetting.GetTags()
		value, err := protojson.Marshal(tagsUserSetting)
		if err != nil {
			return nil, errors.Wrap(err, "marshal tags user setting")
		}
		raw.Value = string(value)
	case storepb.UserSetting_EDITOR:
		value, err := protojson.Marshal(userSetting.GetEditor())
		if err != nil {
			return nil, errors.Wrap(err, "marshal editor user setting")
		}
		raw.Value = string(value)
	case storepb.UserSetting_REFRESH_TOKENS:
		refreshTokensUserSetting := userSetting.GetRefreshTokens()
		value, err := protojson.Marshal(refreshTokensUserSetting)
		if err != nil {
			return nil, err
		}
		raw.Value = string(value)
	case storepb.UserSetting_PERSONAL_ACCESS_TOKENS:
		patsUserSetting := userSetting.GetPersonalAccessTokens()
		value, err := protojson.Marshal(patsUserSetting)
		if err != nil {
			return nil, err
		}
		raw.Value = string(value)
	case storepb.UserSetting_WEBHOOKS:
		webhooksUserSetting := userSetting.GetWebhooks()
		value, err := protojson.Marshal(webhooksUserSetting)
		if err != nil {
			return nil, err
		}
		raw.Value = string(value)
	default:
		return nil, errors.Errorf("unsupported user setting key: %v", userSetting.Key)
	}
	return raw, nil
}
