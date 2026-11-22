package messagecapture

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/version"
	"github.com/usememos/memos/store"
	"github.com/usememos/memos/store/db/sqlite"
)

const testSecret = "instance-key-for-message-capture-tests"

func newTestStore(t *testing.T, dataPath string) *store.Store {
	t.Helper()
	p := &profile.Profile{Data: dataPath, Driver: "sqlite", Version: version.GetCurrentVersion()}
	require.NoError(t, p.Validate())
	driver, err := sqlite.NewDB(p)
	require.NoError(t, err)
	s := store.New(driver, p)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	require.NoError(t, s.Migrate(t.Context()))
	return s
}

func TestNotificationSettingsPreserveChannelFallbackAndSecrets(t *testing.T) {
	ctx := t.Context()
	dataPath := t.TempDir()
	s := newTestStore(t, dataPath)
	r := NewRepository(s, testSecret)
	settingsBefore, err := s.ListInstanceSettings(ctx, &store.FindInstanceSetting{})
	require.NoError(t, err)
	aesKey := base64.RawStdEncoding.EncodeToString([]byte(strings.Repeat("z", 32)))
	environment := map[string]string{
		"CAT_TWO_TELEGRAM_USER_ID":       " 456 ",
		"CAT_TWO_TELEGRAM_BOT_TOKEN":     "123:previous-secret",
		"CAT_TWO_WECOM_CORP_ID":          "corp",
		"CAT_TWO_WECOM_AGENT_ID":         "42",
		"CAT_TWO_WECOM_USER_ID":          "member",
		"CAT_TWO_WECOM_CALLBACK_TOKEN":   "callback-secret",
		"CAT_TWO_WECOM_ENCODING_AES_KEY": aesKey,
	}
	fallback := FromEnvironment(func(key string) string { return environment[key] })
	initial, err := r.Load(ctx, fallback)
	require.NoError(t, err)
	require.Equal(t, SourceEnvironment, initial.TelegramSource)
	require.Equal(t, SourceEnvironment, initial.WeComSource)
	require.True(t, initial.Telegram.Enabled)
	require.True(t, initial.WeCom.Enabled)

	updated, err := r.Save(ctx, fallback, Update{Telegram: &Telegram{UserID: " 456 "}})
	require.NoError(t, err)
	require.Equal(t, SourceSettings, updated.TelegramSource)
	require.Equal(t, SourceEnvironment, updated.WeComSource)
	require.False(t, updated.Telegram.Enabled)
	require.True(t, updated.Telegram.Configured())
	require.Equal(t, "123:previous-secret", updated.Telegram.BotToken)

	changed := FromEnvironment(func(key string) string {
		if key == "CAT_TWO_WECOM_USER_ID" {
			return "another-member"
		}
		return environment[key]
	})
	updated, err = r.Load(ctx, changed)
	require.NoError(t, err)
	require.Equal(t, "another-member", updated.WeCom.UserID)
	require.False(t, updated.Telegram.Enabled)
	updated, err = r.Save(ctx, changed, Update{WeCom: &WeCom{Enabled: true, CorpID: "corp", AgentID: "42", UserID: "another-member"}})
	require.NoError(t, err)
	require.Equal(t, SourceSettings, updated.WeComSource)
	require.Equal(t, aesKey, updated.WeCom.EncodingAESKey)

	// A new repository reads the values from the same SQLite data directory without environment variables.
	reopenedStore := newTestStore(t, dataPath)
	reopened := NewRepository(reopenedStore, testSecret)
	persisted, err := reopened.Load(ctx, Configuration{})
	require.NoError(t, err)
	require.Equal(t, updated, persisted)
	sealed, err := reopenedStore.GetMessageCaptureSettings(ctx)
	require.NoError(t, err)
	require.NotNil(t, sealed)
	require.NotContains(t, *sealed, "previous-secret")
	require.NotContains(t, *sealed, "callback-secret")
	require.NotContains(t, *sealed, aesKey)
	listed, err := reopenedStore.ListInstanceSettings(ctx, &store.FindInstanceSetting{})
	require.NoError(t, err)
	require.ElementsMatch(t, settingsBefore, listed)

	_, err = NewRepository(reopenedStore, "incorrect-instance-secret").Load(ctx, changed)
	require.ErrorContains(t, err, "failed to authenticate encrypted external inbox settings")
}

func TestConcurrentNotificationAndNetworkSavesKeepEachSetting(t *testing.T) {
	ctx := t.Context()
	s := newTestStore(t, t.TempDir())
	first := NewRepository(s, testSecret)
	second := NewRepository(s, testSecret)
	unset, err := first.LoadNetwork(ctx)
	require.NoError(t, err)
	require.Equal(t, NetworkConfiguration{}, unset)

	start := make(chan struct{})
	errs := make(chan error, 2)
	proxyURL := "socks5h://proxy-user:proxy-password@[2001:db8::1]:1080"
	go func() {
		<-start
		_, err := first.Save(ctx, Configuration{}, Update{Telegram: &Telegram{Enabled: true, UserID: "123", BotToken: "456:bot-secret"}})
		errs <- err
	}()
	go func() {
		<-start
		_, err := second.SaveNetwork(ctx, NetworkUpdate{Enabled: true, ProxyURL: proxyURL})
		errs <- err
	}()
	close(start)
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	config, err := first.Load(ctx, Configuration{})
	require.NoError(t, err)
	require.True(t, config.Telegram.Enabled)
	require.Equal(t, "456:bot-secret", config.Telegram.BotToken)
	network, err := first.LoadNetwork(ctx)
	require.NoError(t, err)
	require.Equal(t, NetworkConfiguration{Enabled: true, ProxyURL: proxyURL, Source: SourceSettings}, network)

	disabled, err := second.SaveNetwork(ctx, NetworkUpdate{})
	require.NoError(t, err)
	require.Equal(t, NetworkConfiguration{ProxyURL: proxyURL, Source: SourceSettings}, disabled)
	reenabled, err := second.SaveNetwork(ctx, NetworkUpdate{Enabled: true})
	require.NoError(t, err)
	require.Equal(t, network, reenabled)
	sealed, err := s.GetMessageCaptureSettings(ctx)
	require.NoError(t, err)
	require.NotNil(t, sealed)
	require.NotContains(t, *sealed, "proxy-password")
}

func TestInvalidConfigurationErrorsHideInput(t *testing.T) {
	ctx := t.Context()
	r := NewRepository(newTestStore(t, t.TempDir()), testSecret)
	_, err := r.Save(ctx, Configuration{}, Update{Telegram: &Telegram{Enabled: true, UserID: "123", BotToken: "not-a-token"}})
	require.ErrorIs(t, err, ErrInvalidConfiguration)
	require.NotContains(t, err.Error(), "not-a-token")
	_, err = r.Save(ctx, Configuration{}, Update{})
	require.ErrorIs(t, err, ErrInvalidConfiguration)
	_, err = r.Save(ctx, Configuration{}, Update{Telegram: &Telegram{}, WeCom: &WeCom{}})
	require.ErrorIs(t, err, ErrInvalidConfiguration)
	for _, invalid := range []string{
		"",
		"socks4://proxy:1080",
		"http://proxy/path",
		"http://proxy?",
		"http://proxy#",
		"http://proxy:",
		"http://proxy:123:456",
		"http://user:proxy-secret@proxy:65536",
		"http://:proxy-secret@proxy",
		"http://[not-an-ip]",
	} {
		_, err := r.SaveNetwork(ctx, NetworkUpdate{Enabled: true, ProxyURL: invalid})
		require.ErrorIs(t, err, ErrInvalidConfiguration)
		require.NotContains(t, err.Error(), "proxy-secret")
	}
	require.Equal(t, Configuration{}, FromEnvironment(func(string) string { return "" }))
	partial := FromEnvironment(func(key string) string {
		if key == "CAT_TWO_TELEGRAM_USER_ID" {
			return "42"
		}
		return ""
	})
	require.Equal(t, SourceEnvironment, partial.TelegramSource)
	require.False(t, partial.Telegram.Enabled)
}
