// Package messagecapture manages the configuration for Cat_two's external inboxes.
package messagecapture

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// ErrInvalidConfiguration indicates a configuration value an administrator can correct.
var ErrInvalidConfiguration = errors.New("invalid notification or network settings")

// Source identifies where a channel or proxy configuration came from.
type Source string

const (
	// SourceEnvironment identifies a channel configured by the container environment.
	SourceEnvironment Source = "environment"
	// SourceSettings identifies a configuration saved by an administrator.
	SourceSettings Source = "settings"
)

// Telegram configures one Telegram bot and the authorized private-chat sender.
type Telegram struct {
	Enabled  bool   `json:"enabled"`
	UserID   string `json:"userId"`
	BotToken string `json:"botToken,omitempty"`
}

// Configured reports whether the channel has all the fields it needs to run.
func (c Telegram) Configured() bool {
	return c.validate() == nil
}

func (c Telegram) validate() error {
	if !positiveID(c.UserID) {
		return errors.Wrap(ErrInvalidConfiguration, "Telegram user ID must be a positive integer")
	}
	bot, secret, ok := strings.Cut(c.BotToken, ":")
	botID, err := strconv.ParseUint(bot, 10, 64)
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
	if !ok || err != nil || botID == 0 || secret == "" || len(c.BotToken) > 128 ||
		strings.ContainsFunc(secret, func(r rune) bool { return !strings.ContainsRune(alphabet, r) }) {
		return errors.Wrap(ErrInvalidConfiguration, "Telegram requires a valid BotFather token")
	}
	return nil
}

// WeCom configures one self-built WeCom application and the authorized sender.
type WeCom struct {
	Enabled        bool   `json:"enabled"`
	CorpID         string `json:"corpId"`
	AgentID        string `json:"agentId"`
	UserID         string `json:"userId"`
	CallbackToken  string `json:"callbackToken,omitempty"`
	EncodingAESKey string `json:"encodingAesKey,omitempty"`
	CallbackURL    string `json:"callbackUrl,omitempty"`
}

// Configured reports whether the channel has all the fields it needs to run.
func (c WeCom) Configured() bool {
	return c.validate() == nil
}

func (c WeCom) validate() error {
	if c.CorpID == "" || c.CorpID != strings.TrimSpace(c.CorpID) || strings.Contains(c.CorpID, "/") {
		return errors.Wrap(ErrInvalidConfiguration, "WeCom requires a valid corporation ID")
	}
	if !positiveID(c.AgentID) {
		return errors.Wrap(ErrInvalidConfiguration, "WeCom application AgentID must be a positive integer")
	}
	if c.UserID == "" || c.UserID != strings.TrimSpace(c.UserID) || strings.Contains(c.UserID, "/") {
		return errors.Wrap(ErrInvalidConfiguration, "WeCom requires a valid sender user ID")
	}
	if c.CallbackToken == "" || c.CallbackToken != strings.TrimSpace(c.CallbackToken) {
		return errors.Wrap(ErrInvalidConfiguration, "WeCom requires a callback token")
	}
	if len(c.EncodingAESKey) != 43 {
		return errors.Wrap(ErrInvalidConfiguration, "WeCom requires a 43-character EncodingAESKey")
	}
	key, err := base64.RawStdEncoding.Strict().DecodeString(c.EncodingAESKey)
	if err != nil || len(key) != 32 {
		return errors.Wrap(ErrInvalidConfiguration, "WeCom requires a valid EncodingAESKey")
	}
	return nil
}

func positiveID(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0
}

// Update replaces the selected channel configuration. Empty secrets keep their current values.
type Update struct {
	Telegram *Telegram    `json:"telegram,omitempty"`
	WeCom    *WeComUpdate `json:"wecom,omitempty"`
}

// WeComUpdate configures one WeCom application. An omitted CallbackURL keeps
// the saved address, and an empty CallbackURL clears it.
type WeComUpdate struct {
	Enabled        bool    `json:"enabled"`
	CorpID         string  `json:"corpId"`
	AgentID        string  `json:"agentId"`
	UserID         string  `json:"userId"`
	CallbackToken  string  `json:"callbackToken,omitempty"`
	EncodingAESKey string  `json:"encodingAesKey,omitempty"`
	CallbackURL    *string `json:"callbackUrl,omitempty"`
}

// Configuration describes the effective configuration of both external inboxes.
type Configuration struct {
	Telegram       Telegram
	WeCom          WeCom
	TelegramSource Source
	WeComSource    Source
}

// FromEnvironment reads the original container configuration used for channels without saved settings.
func FromEnvironment(getenv func(string) string) Configuration {
	telegram := Telegram{
		UserID:   strings.TrimSpace(getenv("CAT_TWO_TELEGRAM_USER_ID")),
		BotToken: strings.TrimSpace(getenv("CAT_TWO_TELEGRAM_BOT_TOKEN")),
	}
	wecom := WeCom{
		CorpID:         strings.TrimSpace(getenv("CAT_TWO_WECOM_CORP_ID")),
		AgentID:        strings.TrimSpace(getenv("CAT_TWO_WECOM_AGENT_ID")),
		UserID:         strings.TrimSpace(getenv("CAT_TWO_WECOM_USER_ID")),
		CallbackToken:  strings.TrimSpace(getenv("CAT_TWO_WECOM_CALLBACK_TOKEN")),
		EncodingAESKey: strings.TrimSpace(getenv("CAT_TWO_WECOM_ENCODING_AES_KEY")),
	}
	var config Configuration
	if telegram != (Telegram{}) {
		telegram.Enabled = telegram.Configured()
		config.Telegram = telegram
		config.TelegramSource = SourceEnvironment
	}
	if wecom != (WeCom{}) {
		wecom.Enabled = wecom.Configured()
		config.WeCom = wecom
		config.WeComSource = SourceEnvironment
	}
	return config
}

func (u Update) apply(saved *savedConfiguration, current Configuration) error {
	if (u.Telegram == nil) == (u.WeCom == nil) {
		return errors.Wrap(ErrInvalidConfiguration, "select exactly one notification channel")
	}
	if u.Telegram != nil {
		telegram := *u.Telegram
		telegram.UserID = strings.TrimSpace(telegram.UserID)
		telegram.BotToken = secretOrCurrent(telegram.BotToken, current.Telegram.BotToken)
		if telegram.Enabled {
			if err := telegram.validate(); err != nil {
				return err
			}
		}
		saved.Telegram = &telegram
		return nil
	}
	wecom := WeCom{
		Enabled:        u.WeCom.Enabled,
		CorpID:         strings.TrimSpace(u.WeCom.CorpID),
		AgentID:        strings.TrimSpace(u.WeCom.AgentID),
		UserID:         strings.TrimSpace(u.WeCom.UserID),
		CallbackToken:  secretOrCurrent(u.WeCom.CallbackToken, current.WeCom.CallbackToken),
		EncodingAESKey: secretOrCurrent(u.WeCom.EncodingAESKey, current.WeCom.EncodingAESKey),
		CallbackURL:    current.WeCom.CallbackURL,
	}
	if u.WeCom.CallbackURL != nil {
		callbackURL, err := NormalizeWeComCallbackURL(*u.WeCom.CallbackURL)
		if err != nil {
			return err
		}
		wecom.CallbackURL = callbackURL
	}
	if wecom.Enabled {
		if err := wecom.validate(); err != nil {
			return err
		}
	}
	saved.WeCom = &wecom
	return nil
}

func secretOrCurrent(value, current string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return current
}
