package instancesetting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	storepb "github.com/usememos/memos/proto/gen/store"
)

func TestValidateRequiresValueMatchingKey(t *testing.T) {
	for _, key := range []storepb.InstanceSettingKey{
		storepb.InstanceSettingKey_GENERAL,
		storepb.InstanceSettingKey_TAGS,
		storepb.InstanceSettingKey_NOTIFICATION,
		storepb.InstanceSettingKey_AI,
		storepb.InstanceSettingKey_ACCESS,
		storepb.InstanceSettingKey_USAGE_LIMITS,
	} {
		// A GENERAL value under every other key is a mismatch; under GENERAL
		// the TAGS value is.
		setting := &storepb.InstanceSetting{
			Key:   key,
			Value: &storepb.InstanceSetting_GeneralSetting{GeneralSetting: &storepb.InstanceGeneralSetting{}},
		}
		if key == storepb.InstanceSettingKey_GENERAL {
			setting.Value = &storepb.InstanceSetting_TagsSetting{TagsSetting: &storepb.InstanceTagsSetting{}}
		}
		require.Error(t, Validate(setting), key.String())
	}
}

func TestValidateRejectsUnwritableKeys(t *testing.T) {
	err := Validate(&storepb.InstanceSetting{
		Key:   storepb.InstanceSettingKey_BASIC,
		Value: &storepb.InstanceSetting_BasicSetting{BasicSetting: &storepb.InstanceBasicSetting{}},
	})
	require.ErrorContains(t, err, "cannot be written")
	require.ErrorContains(t, Validate(&storepb.InstanceSetting{Key: storepb.InstanceSettingKey_CONFIGURATION_STATE}), "state record")
	require.ErrorContains(t, Validate(&storepb.InstanceSetting{}), "unsupported instance setting key")
	require.Error(t, Validate(nil))
}

func TestValidateGeneral(t *testing.T) {
	general := func(offset int32) *storepb.InstanceSetting {
		return &storepb.InstanceSetting{
			Key:   storepb.InstanceSettingKey_GENERAL,
			Value: &storepb.InstanceSetting_GeneralSetting{GeneralSetting: &storepb.InstanceGeneralSetting{WeekStartDayOffset: offset}},
		}
	}
	require.NoError(t, Validate(general(-1)))
	require.NoError(t, Validate(general(6)))
	require.ErrorContains(t, Validate(general(7)), "week_start_day_offset")
	require.ErrorContains(t, Validate(general(-2)), "week_start_day_offset")
}

func TestValidateAccess(t *testing.T) {
	access := func(mode storepb.InstanceAccessMode) *storepb.InstanceSetting {
		return &storepb.InstanceSetting{
			Key:   storepb.InstanceSettingKey_ACCESS,
			Value: &storepb.InstanceSetting_AccessSetting{AccessSetting: &storepb.InstanceAccessSetting{AccessMode: mode}},
		}
	}
	require.NoError(t, Validate(access(storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE)))
	require.NoError(t, Validate(access(storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PUBLIC)))
	require.ErrorContains(t, Validate(access(storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_UNSPECIFIED)), "access_mode")
}

func TestValidateNotification(t *testing.T) {
	notification := func(email *storepb.InstanceNotificationSetting_EmailSetting) *storepb.InstanceSetting {
		return &storepb.InstanceSetting{
			Key:   storepb.InstanceSettingKey_NOTIFICATION,
			Value: &storepb.InstanceSetting_NotificationSetting{NotificationSetting: &storepb.InstanceNotificationSetting{Email: email}},
		}
	}
	require.NoError(t, Validate(notification(nil)))
	require.NoError(t, Validate(notification(&storepb.InstanceNotificationSetting_EmailSetting{Enabled: false})))
	require.NoError(t, Validate(notification(&storepb.InstanceNotificationSetting_EmailSetting{
		Enabled: true, SmtpHost: "smtp.example.com", SmtpPort: 587, FromEmail: "memos@example.com", UseTls: true,
	})))
	require.ErrorContains(t, Validate(notification(&storepb.InstanceNotificationSetting_EmailSetting{
		Enabled: true, SmtpHost: "smtp.example.com", FromEmail: "memos@example.com",
	})), "smtp_port")
	require.ErrorContains(t, Validate(notification(&storepb.InstanceNotificationSetting_EmailSetting{
		Enabled: true, SmtpHost: "smtp.example.com", SmtpPort: 465, FromEmail: "memos@example.com", UseTls: true, UseSsl: true,
	})), "use_tls and use_ssl")
}

func aiSetting(providers ...*storepb.AIProviderConfig) *storepb.InstanceSetting {
	return &storepb.InstanceSetting{
		Key:   storepb.InstanceSettingKey_AI,
		Value: &storepb.InstanceSetting_AiSetting{AiSetting: &storepb.InstanceAISetting{Providers: providers}},
	}
}

func TestValidateAI(t *testing.T) {
	provider := func(id string) *storepb.AIProviderConfig {
		return &storepb.AIProviderConfig{Id: id, Title: "OpenAI", Type: storepb.AIProviderType_OPENAI, ApiKey: "key"}
	}
	require.NoError(t, Validate(aiSetting()))
	require.NoError(t, Validate(aiSetting(provider("a"), provider("b"))))
	require.ErrorContains(t, Validate(aiSetting(provider(""))), "id is required")
	require.ErrorContains(t, Validate(aiSetting(provider("a"), provider("a"))), "duplicate provider ID")

	noKey := provider("a")
	noKey.ApiKey = ""
	require.ErrorContains(t, Validate(aiSetting(noKey)), "API key is required")

	badType := provider("a")
	badType.Type = storepb.AIProviderType_AI_PROVIDER_TYPE_UNSPECIFIED
	require.ErrorContains(t, Validate(aiSetting(badType)), "unsupported type")

	setting := aiSetting(provider("a"))
	setting.GetAiSetting().Transcription = &storepb.TranscriptionConfig{ProviderId: "missing"}
	require.ErrorContains(t, Validate(setting), "does not reference")
	setting.GetAiSetting().Transcription = &storepb.TranscriptionConfig{ProviderId: "a", Language: strings.Repeat("x", MaxTranscriptionLanguageLength+1)}
	require.ErrorContains(t, Validate(setting), "language is too long")
}

func TestNormalizeAISetting(t *testing.T) {
	setting := &storepb.InstanceAISetting{
		Providers: []*storepb.AIProviderConfig{
			{Id: " a ", Title: " OpenAI ", Type: storepb.AIProviderType_OPENAI},
			{Id: "b", Type: storepb.AIProviderType_GEMINI, Endpoint: " https://proxy.example.com "},
			nil,
		},
		Transcription: &storepb.TranscriptionConfig{ProviderId: " a ", Model: " whisper-1 "},
	}
	NormalizeAISetting(setting)
	require.Equal(t, "a", setting.Providers[0].Id)
	require.Equal(t, "OpenAI", setting.Providers[0].Title)
	require.Equal(t, DefaultOpenAIEndpoint, setting.Providers[0].Endpoint)
	require.Equal(t, "https://proxy.example.com", setting.Providers[1].Endpoint)
	require.Equal(t, "a", setting.Transcription.ProviderId)
	require.Equal(t, "whisper-1", setting.Transcription.Model)
}

func TestValidateManagedStorageCannotBeWritten(t *testing.T) {
	require.ErrorContains(t, Validate(&storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_STORAGE,
		Value: &storepb.InstanceSetting_StorageSetting{StorageSetting: &storepb.InstanceStorageSetting{
			StorageType: storepb.InstanceStorageSetting_LOCAL,
		}},
	}), "attachment storage is managed by Cat_two")
}
