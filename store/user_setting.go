package store

import (
	"context"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

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
	userSettingRawList, err := s.driver.ListUserSettings(ctx, nil, find)
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

// Credential reads and mutations use the database row so that a cached snapshot
// cannot hide a revocation or restore a credential that has already been revoked.
func (s *Store) getUserCredentialSetting(ctx context.Context, userID int32, key storepb.UserSetting_Key) (*storepb.UserSetting, error) {
	settings, err := s.driver.ListUserSettings(ctx, nil, &FindUserSetting{UserID: &userID, Key: key})
	if err != nil || len(settings) == 0 {
		return nil, err
	}
	if len(settings) > 1 {
		return nil, errors.Errorf("expected 1 user setting, but got %d", len(settings))
	}
	return convertUserSettingFromRaw(settings[0])
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
	default:
		return nil, errors.Errorf("unsupported user setting key: %v", userSetting.Key)
	}
	return raw, nil
}
