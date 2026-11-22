package store

import "context"

// This internal row holds only encrypted data and is omitted from the generic instance settings API.
const messageCaptureSettingKey = "CAT_TWO_MESSAGE_CAPTURE"

// GetMessageCaptureSettings returns the encrypted external inbox settings, or nil if absent.
func (s *Store) GetMessageCaptureSettings(ctx context.Context) (*string, error) {
	settings, err := s.driver.ListInstanceSettings(ctx, &FindInstanceSetting{Name: messageCaptureSettingKey})
	if err != nil || len(settings) == 0 {
		return nil, err
	}
	return &settings[0].Value, nil
}

// CompareAndSwapMessageCaptureSettings saves a new encrypted value if the previous value still matches.
// A nil previous value expects the row to be absent.
func (s *Store) CompareAndSwapMessageCaptureSettings(ctx context.Context, previous *string, replacement string) (bool, error) {
	if previous == nil {
		return s.driver.CreateInstanceSettingIfNotExists(ctx, &InstanceSetting{Name: messageCaptureSettingKey, Value: replacement})
	}
	return s.driver.UpdateInstanceSettingIfUnchanged(ctx, messageCaptureSettingKey, *previous, replacement)
}
