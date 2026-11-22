package messagecapture

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/pkg/errors"

	"github.com/usememos/memos/store"
)

const (
	settingsKeyPurpose = "cat-two/message-capture-settings/v1"
	settingsVersion    = "v1."
)

type savedConfiguration struct {
	Update
	Network *NetworkUpdate `json:"network,omitempty"`
}

func (s savedConfiguration) resolve(fallback Configuration) Configuration {
	if s.Telegram != nil {
		fallback.Telegram = *s.Telegram
		fallback.TelegramSource = SourceSettings
	}
	if s.WeCom != nil {
		fallback.WeCom = *s.WeCom
		fallback.WeComSource = SourceSettings
	}
	return fallback
}

func (s savedConfiguration) network() NetworkConfiguration {
	if s.Network == nil {
		return NetworkConfiguration{}
	}
	return NetworkConfiguration{Enabled: s.Network.Enabled, ProxyURL: s.Network.ProxyURL, Source: SourceSettings}
}

// Repository stores notification and network settings encrypted with the instance secret.
type Repository struct {
	store          *store.Store
	instanceSecret string
}

// NewRepository creates a repository scoped to this Cat_two instance.
func NewRepository(s *store.Store, instanceSecret string) *Repository {
	return &Repository{store: s, instanceSecret: instanceSecret}
}

// Load resolves saved channel settings over the original environment configuration.
func (r *Repository) Load(ctx context.Context, fallback Configuration) (Configuration, error) {
	saved, _, err := r.read(ctx)
	if err != nil {
		return Configuration{}, err
	}
	return saved.resolve(fallback), nil
}

// Save updates one channel and returns the effective configuration after the update.
func (r *Repository) Save(ctx context.Context, fallback Configuration, update Update) (Configuration, error) {
	saved, err := r.update(ctx, func(saved *savedConfiguration) error {
		return update.apply(saved, saved.resolve(fallback))
	})
	if err != nil {
		return Configuration{}, err
	}
	return saved.resolve(fallback), nil
}

// LoadNetwork reads the saved network settings, if any.
func (r *Repository) LoadNetwork(ctx context.Context) (NetworkConfiguration, error) {
	saved, _, err := r.read(ctx)
	if err != nil {
		return NetworkConfiguration{}, err
	}
	return saved.network(), nil
}

// SaveNetwork persists the Telegram proxy settings.
func (r *Repository) SaveNetwork(ctx context.Context, update NetworkUpdate) (NetworkConfiguration, error) {
	saved, err := r.update(ctx, update.apply)
	if err != nil {
		return NetworkConfiguration{}, err
	}
	return saved.network(), nil
}

func (r *Repository) update(ctx context.Context, apply func(*savedConfiguration) error) (savedConfiguration, error) {
	for ctx.Err() == nil {
		saved, previous, err := r.read(ctx)
		if err != nil {
			return savedConfiguration{}, err
		}
		if err := apply(&saved); err != nil {
			return savedConfiguration{}, err
		}
		sealed, err := r.seal(saved)
		if err != nil {
			return savedConfiguration{}, err
		}
		changed, err := r.store.CompareAndSwapMessageCaptureSettings(ctx, previous, sealed)
		if err != nil {
			return savedConfiguration{}, errors.Wrap(err, "save external inbox settings")
		}
		if changed {
			return saved, nil
		}
	}
	return savedConfiguration{}, ctx.Err()
}

func (r *Repository) read(ctx context.Context) (savedConfiguration, *string, error) {
	var saved savedConfiguration
	previous, err := r.store.GetMessageCaptureSettings(ctx)
	if err != nil {
		return saved, nil, errors.Wrap(err, "read external inbox settings")
	}
	if previous == nil {
		return saved, nil, nil
	}
	encoded, ok := strings.CutPrefix(*previous, settingsVersion)
	if !ok {
		return saved, nil, errors.New("unsupported external inbox settings version")
	}
	aead, err := r.settingsCipher()
	if err != nil {
		return saved, nil, err
	}
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return saved, nil, errors.New("invalid encrypted external inbox settings")
	}
	plain, err := aead.Open(nil, nil, sealed, []byte(settingsKeyPurpose))
	if err != nil {
		return saved, nil, errors.New("failed to authenticate encrypted external inbox settings")
	}
	if err := json.Unmarshal(plain, &saved); err != nil {
		return savedConfiguration{}, nil, errors.New("invalid external inbox settings")
	}
	return saved, previous, nil
}

func (r *Repository) seal(saved savedConfiguration) (string, error) {
	aead, err := r.settingsCipher()
	if err != nil {
		return "", err
	}
	plain, err := json.Marshal(saved)
	if err != nil {
		return "", errors.New("failed to encode external inbox settings")
	}
	sealed := aead.Seal(nil, nil, plain, []byte(settingsKeyPurpose))
	return settingsVersion + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (r *Repository) settingsCipher() (cipher.AEAD, error) {
	if r.instanceSecret == "" {
		return nil, errors.New("external inbox settings require an instance secret")
	}
	key := hmac.New(sha256.New, []byte(r.instanceSecret))
	key.Write([]byte(settingsKeyPurpose))
	block, err := aes.NewCipher(key.Sum(nil))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}
