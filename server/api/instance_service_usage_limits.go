package api

import (
	"context"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/core/instancesetting"
	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
)

// convertInstanceSettingResponse converts a stored setting for the API. Usage
// limits name users, so they need the store to turn IDs into user names.
func (s *APIService) convertInstanceSettingResponse(ctx context.Context, setting *storepb.InstanceSetting) (*apipb.InstanceSetting, error) {
	result := convertInstanceSettingFromStore(setting)
	if setting.Key == storepb.InstanceSettingKey_USAGE_LIMITS {
		usageLimits, err := s.convertUsageLimitsSettingFromStore(ctx, setting.GetUsageLimitsSetting())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to convert usage limits: %v", err)
		}
		result.Value = &apipb.InstanceSetting_UsageLimitsSetting_{UsageLimitsSetting: usageLimits}
	}
	return result, nil
}

// convertUsageLimitsSettingFromStore converts the stored usage limits for the
// API, naming users by username. Users that no longer exist are left out,
// because an entry naming only missing users never matches anyone.
func (s *APIService) convertUsageLimitsSettingFromStore(ctx context.Context, setting *storepb.InstanceUsageLimitsSetting) (*apipb.InstanceSetting_UsageLimitsSetting, error) {
	userIDs := []int32{}
	for _, limit := range setting.GetLimits() {
		userIDs = append(userIDs, limit.GetUserIds()...)
	}
	usernames, err := s.listUsernamesByID(ctx, userIDs)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list users")
	}
	userNames := make(map[int32]string, len(usernames))
	for userID, username := range usernames {
		userNames[userID] = BuildUserName(username)
	}
	return instancesetting.UsageLimitsFromStore(setting, userNames), nil
}
