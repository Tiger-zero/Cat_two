package api

import (
	"github.com/usememos/memos/core/instancesetting"
	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

// The conversion between API and store settings lives in core/instancesetting,
// because declarative configuration files use the same API messages. This file
// holds what only responses need.

// convertInstanceSettingFromStore converts a stored setting for a response,
// with write-only secrets cleared.
func convertInstanceSettingFromStore(setting *storepb.InstanceSetting) *apipb.InstanceSetting {
	if setting.GetKey() == storepb.InstanceSettingKey_STORAGE {
		return &apipb.InstanceSetting{
			Name: instancesetting.NamePrefix + setting.Key.String(),
			Value: &apipb.InstanceSetting_StorageSetting_{StorageSetting: &apipb.InstanceSetting_StorageSetting{
				StorageType:      apipb.InstanceSetting_StorageSetting_LOCAL,
				FilepathTemplate: store.AttachmentDirectory + "/{timestamp}_{uuid}_{filename}",
				DefaultStorageId: "local",
				Storages: []*apipb.InstanceSetting_Storage{{
					Id: "local", Name: "Local", Type: apipb.InstanceSetting_LOCAL,
				}},
			}},
		}
	}
	result := instancesetting.FromStore(setting)
	redactInstanceSetting(result)
	return result
}

// redactInstanceSetting clears the SMTP password and replaces
// each AI API key with whether it is set and a masked hint.
func redactInstanceSetting(setting *apipb.InstanceSetting) {
	if email := setting.GetNotificationSetting().GetEmail(); email != nil {
		email.SmtpPassword = ""
	}
	for _, provider := range setting.GetAiSetting().GetProviders() {
		provider.ApiKeySet = provider.ApiKey != ""
		provider.ApiKeyHint = maskAPIKey(provider.ApiKey)
		provider.ApiKey = ""
	}
}
