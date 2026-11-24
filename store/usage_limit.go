package store

import (
	"context"
	"slices"

	"github.com/pkg/errors"

	storepb "github.com/usememos/memos/proto/gen/store"
)

// MaxUsageLimitEntries is the most entries the usage limits setting can hold.
const MaxUsageLimitEntries = 100

// MaxUsageLimitUsers is the most user IDs the usage limits setting can name
// across all entries. All entries share one system_setting row, so the cap
// also bounds that row's size.
const MaxUsageLimitUsers = 1000

// DefaultUploadBytes is the built-in per-file upload limit in bytes. 32 MiB.
const DefaultUploadBytes int64 = 32 << 20

// DefaultMemoContentBytes is the built-in per-memo content limit in bytes. 8 KiB.
const DefaultMemoContentBytes int64 = 8 << 10

// ErrInvalidUsageLimits indicates that a legacy setting breaks a validation rule.
var ErrInvalidUsageLimits = errors.New("invalid usage limits")

// DefaultInstanceUsageLimitsSetting is the setting in effect when none is
// stored: one entry for every user that limits memo content to 8 KiB and
// uploads to 32 MiB.
func DefaultInstanceUsageLimitsSetting() *storepb.InstanceUsageLimitsSetting {
	uploadBytes, contentBytes := DefaultUploadBytes, DefaultMemoContentBytes
	return &storepb.InstanceUsageLimitsSetting{Limits: []*storepb.InstanceUsageLimitsSetting_UsageLimit{{
		AllUsers:         true,
		MemoContentBytes: &contentBytes,
		UploadBytes:      &uploadBytes,
	}}}
}

// ValidateInstanceUsageLimitsSetting checks the rules every stored usage
// limits setting meets, whether it comes from the API or from a deployment
// file: at most MaxUsageLimitEntries entries naming at most MaxUsageLimitUsers
// users, every entry names positive, unrepeated users or all users but not
// both, the last entry and only the last entry names all users, and no limit
// is negative.
func ValidateInstanceUsageLimitsSetting(setting *storepb.InstanceUsageLimitsSetting) error {
	if setting == nil {
		return errors.Wrap(ErrInvalidUsageLimits, "usage limits must be populated")
	}
	limits := setting.GetLimits()
	if len(limits) == 0 {
		return errors.Wrap(ErrInvalidUsageLimits, "the last entry must apply to all users")
	}
	if len(limits) > MaxUsageLimitEntries {
		return errors.Wrapf(ErrInvalidUsageLimits, "at most %d entries are allowed", MaxUsageLimitEntries)
	}
	userCount := 0
	for i, limit := range limits {
		if limit == nil {
			return errors.Wrapf(ErrInvalidUsageLimits, "limits[%d] must not be null", i)
		}
		last := i == len(limits)-1
		switch {
		case limit.AllUsers && len(limit.UserIds) > 0:
			return errors.Wrapf(ErrInvalidUsageLimits, "limits[%d] names users and all users; choose one", i)
		case limit.AllUsers && !last:
			return errors.Wrapf(ErrInvalidUsageLimits, "limits[%d] applies to all users, so it must be the last entry", i)
		case !limit.AllUsers && last:
			return errors.Wrap(ErrInvalidUsageLimits, "the last entry must apply to all users")
		case !limit.AllUsers && len(limit.UserIds) == 0:
			return errors.Wrapf(ErrInvalidUsageLimits, "limits[%d] must name at least one user", i)
		}
		// The cap comes before the duplicate scan, so an oversized entry is
		// refused before its quadratic check runs.
		userCount += len(limit.UserIds)
		if userCount > MaxUsageLimitUsers {
			return errors.Wrapf(ErrInvalidUsageLimits, "at most %d users can be named across all entries", MaxUsageLimitUsers)
		}
		for j, userID := range limit.UserIds {
			if userID <= 0 {
				return errors.Wrapf(ErrInvalidUsageLimits, "limits[%d].userIds[%d] must be positive", i, j)
			}
			if slices.Contains(limit.UserIds[:j], userID) {
				return errors.Wrapf(ErrInvalidUsageLimits, "limits[%d] names user %d more than once", i, userID)
			}
		}
		for _, value := range []struct {
			name  string
			value *int64
		}{
			{"memoCount", limit.MemoCount},
			{"memoContentBytes", limit.MemoContentBytes},
			{"storageBytes", limit.StorageBytes},
			{"uploadBytes", limit.UploadBytes},
		} {
			if value.value != nil && *value.value < 0 {
				return errors.Wrapf(ErrInvalidUsageLimits, "limits[%d].%s must not be negative", i, value.name)
			}
		}
	}
	return nil
}

// GetInstanceUsageLimitsSetting returns the usage limits setting. The database
// row is read directly, without the instance setting cache, so a change made
// through any replica applies at the next check on every replica. An absent
// setting is DefaultInstanceUsageLimitsSetting.
func (s *Store) GetInstanceUsageLimitsSetting(ctx context.Context) (*storepb.InstanceUsageLimitsSetting, error) {
	instanceSetting, err := s.GetUncachedInstanceSetting(ctx, &FindInstanceSetting{Name: storepb.InstanceSettingKey_USAGE_LIMITS.String()})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get usage limits setting")
	}
	if setting := instanceSetting.GetUsageLimitsSetting(); setting != nil {
		if err := ValidateInstanceUsageLimitsSetting(setting); err != nil {
			return nil, errors.Wrap(err, "invalid usage limits setting")
		}
		return setting, nil
	}
	return DefaultInstanceUsageLimitsSetting(), nil
}
