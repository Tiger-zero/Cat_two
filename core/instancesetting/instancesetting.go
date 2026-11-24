// Package instancesetting holds the rules an instance setting must meet before
// it is stored, whichever path writes it.
//
// Validate checks the complete value as it would be stored. Conveniences of
// one write path, such as the API treating an empty secret as "keep the stored
// one", run before validation, so the rules here never see a partial value.
package instancesetting

import (
	"strings"

	"github.com/pkg/errors"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

// Length limits of the AI transcription configuration.
const (
	MaxTranscriptionModelLength    = 256
	MaxTranscriptionLanguageLength = 32
	MaxTranscriptionPromptLength   = 4096
)

// Default endpoints applied by NormalizeAISetting when a provider has none.
const (
	DefaultOpenAIEndpoint = "https://api.openai.com/v1"
	DefaultGeminiEndpoint = "https://generativelanguage.googleapis.com/v1beta"
)

// Validate reports the first rule the setting breaks, or nil.
func Validate(setting *storepb.InstanceSetting) error {
	if setting == nil {
		return errors.New("instance setting is required")
	}
	switch setting.Key {
	case storepb.InstanceSettingKey_GENERAL:
		general := setting.GetGeneralSetting()
		if general == nil {
			return errors.New("general setting is required")
		}
		if general.WeekStartDayOffset < -1 || general.WeekStartDayOffset > 6 {
			return errors.New("week_start_day_offset must be between -1 and 6")
		}
	case storepb.InstanceSettingKey_STORAGE:
		return errors.New("attachment storage is managed by Cat_two")
	case storepb.InstanceSettingKey_TAGS:
		if setting.GetTagsSetting() == nil {
			return errors.New("tags setting is required")
		}
	case storepb.InstanceSettingKey_NOTIFICATION:
		if setting.GetNotificationSetting() == nil {
			return errors.New("notification setting is required")
		}
		return validateNotificationSetting(setting.GetNotificationSetting())
	case storepb.InstanceSettingKey_AI:
		if setting.GetAiSetting() == nil {
			return errors.New("AI setting is required")
		}
		return validateAISetting(setting.GetAiSetting())
	case storepb.InstanceSettingKey_ACCESS:
		access := setting.GetAccessSetting()
		if access == nil {
			return errors.New("access setting is required")
		}
		switch access.AccessMode {
		case storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PUBLIC:
		default:
			return errors.New("access_mode must be PRIVATE or PUBLIC")
		}
	case storepb.InstanceSettingKey_USAGE_LIMITS:
		usageLimits := setting.GetUsageLimitsSetting()
		if usageLimits == nil {
			return errors.New("usage limits setting is required")
		}
		return store.ValidateInstanceUsageLimitsSetting(usageLimits)
	case storepb.InstanceSettingKey_BASIC:
		return errors.New("BASIC holds the instance secret and schema version and cannot be written")
	case storepb.InstanceSettingKey_CONFIGURATION_STATE:
		return errors.New("CONFIGURATION_STATE is the declarative configuration state record and cannot be written")
	default:
		return errors.Errorf("unsupported instance setting key %s", setting.Key)
	}
	return nil
}

// validateNotificationSetting requires enabled email to have a host, a
// positive port, and a from address, and keeps TLS and SSL mutually exclusive.
func validateNotificationSetting(setting *storepb.InstanceNotificationSetting) error {
	email := setting.GetEmail()
	if email == nil || !email.Enabled {
		return nil
	}
	if strings.TrimSpace(email.SmtpHost) == "" || email.SmtpPort <= 0 || strings.TrimSpace(email.FromEmail) == "" {
		return errors.New("enabled notification email requires smtp_host, a positive smtp_port, and from_email")
	}
	if email.UseTls && email.UseSsl {
		return errors.New("notification email cannot enable both use_tls and use_ssl")
	}
	return nil
}

// Normalize trims AI providers and fills their default endpoints.
func Normalize(setting *storepb.InstanceSetting) {
	if setting.Key == storepb.InstanceSettingKey_AI {
		NormalizeAISetting(setting.GetAiSetting())
	}
}

// NormalizeAISetting trims provider and transcription fields and fills a
// provider's default endpoint. It never generates IDs or copies secrets.
func NormalizeAISetting(setting *storepb.InstanceAISetting) {
	if setting == nil {
		return
	}
	for _, provider := range setting.Providers {
		if provider == nil {
			continue
		}
		provider.Id = strings.TrimSpace(provider.Id)
		provider.Title = strings.TrimSpace(provider.Title)
		provider.Endpoint = strings.TrimSpace(provider.Endpoint)
		if provider.Endpoint == "" {
			switch provider.Type {
			case storepb.AIProviderType_OPENAI:
				provider.Endpoint = DefaultOpenAIEndpoint
			case storepb.AIProviderType_GEMINI:
				provider.Endpoint = DefaultGeminiEndpoint
			default:
			}
		}
	}
	if transcription := setting.Transcription; transcription != nil {
		transcription.ProviderId = strings.TrimSpace(transcription.ProviderId)
		transcription.Model = strings.TrimSpace(transcription.Model)
		transcription.Language = strings.TrimSpace(transcription.Language)
		transcription.Prompt = strings.TrimSpace(transcription.Prompt)
	}
}

func validateAISetting(setting *storepb.InstanceAISetting) error {
	seenIDs := make(map[string]bool, len(setting.Providers))
	for i, provider := range setting.Providers {
		if provider == nil {
			return errors.Errorf("providers[%d] must not be null", i)
		}
		if provider.Id == "" {
			return errors.Errorf("providers[%d].id is required", i)
		}
		if seenIDs[provider.Id] {
			return errors.Errorf("duplicate provider ID %q", provider.Id)
		}
		seenIDs[provider.Id] = true
		if provider.Title == "" {
			return errors.Errorf("provider %q title is required", provider.Id)
		}
		if provider.Type != storepb.AIProviderType_OPENAI && provider.Type != storepb.AIProviderType_GEMINI {
			return errors.Errorf("provider %q has unsupported type", provider.Id)
		}
		if provider.ApiKey == "" {
			return errors.Errorf("provider %q API key is required", provider.Id)
		}
	}

	transcription := setting.Transcription
	if transcription == nil {
		return nil
	}
	if transcription.ProviderId != "" && !seenIDs[transcription.ProviderId] {
		return errors.Errorf("transcription provider_id %q does not reference any configured provider", transcription.ProviderId)
	}
	if len(transcription.Model) > MaxTranscriptionModelLength {
		return errors.Errorf("transcription model is too long; maximum length is %d characters", MaxTranscriptionModelLength)
	}
	if len(transcription.Language) > MaxTranscriptionLanguageLength {
		return errors.Errorf("transcription language is too long; maximum length is %d characters", MaxTranscriptionLanguageLength)
	}
	if len(transcription.Prompt) > MaxTranscriptionPromptLength {
		return errors.Errorf("transcription prompt is too long; maximum length is %d characters", MaxTranscriptionPromptLength)
	}
	return nil
}
