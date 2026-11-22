package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/usememos/memos/core/messagecapture"
	"github.com/usememos/memos/server/inbox"
)

type messageCapture struct {
	mu            sync.Mutex
	repository    *messagecapture.Repository
	fallback      messagecapture.Configuration
	configuration messagecapture.Configuration
	network       messagecapture.NetworkConfiguration
	saveMemo      func(context.Context, string, string, string) error
	runTelegram   func(context.Context, string, int64, string, func(context.Context, string, string) error)

	lifecycle      context.Context
	telegramCancel context.CancelFunc
	telegramDone   chan struct{}
	weComCallback  atomic.Pointer[http.Handler]
}

func (s *Server) configureMessageCapture(ctx context.Context, mux *http.ServeMux) error {
	capture := &messageCapture{
		saveMemo:    s.apiService.CaptureMemo,
		runTelegram: inbox.RunTelegram,
	}
	if s.Profile.Personal && !s.Profile.Demo {
		capture.repository = messagecapture.NewRepository(s.Store, s.Secret)
		capture.fallback = messagecapture.FromEnvironment(os.Getenv)
		configuration, err := capture.repository.Load(ctx, capture.fallback)
		if err != nil {
			return err
		}
		network, err := capture.repository.LoadNetwork(ctx)
		if err != nil {
			return err
		}
		capture.configuration = configuration
		capture.network = network
		capture.setWeCom(configuration.WeCom)
		if configuration.TelegramSource == messagecapture.SourceEnvironment && !configuration.Telegram.Enabled {
			slog.Warn("Telegram capture disabled: configure a BotFather token and a positive Telegram user ID")
		}
		if configuration.WeComSource == messagecapture.SourceEnvironment && !configuration.WeCom.Enabled {
			slog.Warn("WeCom capture disabled: invalid or incomplete callback configuration")
		}
		s.messageCapture = capture
		s.apiService.NotificationSettings = capture
		s.apiService.NetworkSettings = capture
	}

	// WeCom authenticates with its own signature and encryption.
	mux.HandleFunc(messagecapture.WeComCallbackPath, capture.serveWeCom)
	return nil
}

func (c *messageCapture) GetConfiguration(_ context.Context) (messagecapture.Configuration, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.configuration, nil
}

func (c *messageCapture) UpdateConfiguration(ctx context.Context, update messagecapture.Update) (messagecapture.Configuration, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	next, err := c.repository.Save(ctx, c.fallback, update)
	if err != nil {
		return messagecapture.Configuration{}, err
	}
	if next.WeCom != c.configuration.WeCom {
		c.setWeCom(next.WeCom)
	}
	if next.Telegram != c.configuration.Telegram {
		c.restartTelegram(next.Telegram, telegramProxy(c.network))
	}
	c.configuration = next
	return next, nil
}

func (c *messageCapture) GetNetworkConfiguration(_ context.Context) (messagecapture.NetworkConfiguration, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.network, nil
}

func (c *messageCapture) UpdateNetworkConfiguration(ctx context.Context, update messagecapture.NetworkUpdate) (messagecapture.NetworkConfiguration, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	next, err := c.repository.SaveNetwork(ctx, update)
	if err != nil {
		return messagecapture.NetworkConfiguration{}, err
	}
	if proxy := telegramProxy(next); proxy != telegramProxy(c.network) {
		c.restartTelegram(c.configuration.Telegram, proxy)
	}
	c.network = next
	return next, nil
}

func (c *messageCapture) start(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lifecycle = ctx
	c.restartTelegram(c.configuration.Telegram, telegramProxy(c.network))
}

func (c *messageCapture) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lifecycle = nil
	c.stopTelegram()
}

// The caller holds mu. Waiting for the old poller prevents competing
// getUpdates requests, including when only its proxy or sender changes.
func (c *messageCapture) restartTelegram(setting messagecapture.Telegram, proxy string) {
	c.stopTelegram()
	if !setting.Enabled || c.lifecycle == nil || c.lifecycle.Err() != nil {
		return
	}
	senderID, _ := strconv.ParseInt(setting.UserID, 10, 64)
	botID, _, _ := strings.Cut(setting.BotToken, ":")
	ctx, cancel := context.WithCancel(c.lifecycle)
	done := make(chan struct{})
	c.telegramCancel = cancel
	c.telegramDone = done
	go func() {
		defer close(done)
		c.runTelegram(ctx, setting.BotToken, senderID, proxy, func(ctx context.Context, messageID, text string) error {
			return c.saveMemo(ctx, "telegram", botID+"/"+messageID, text)
		})
	}()
	slog.Info("Telegram capture enabled")
}

func (c *messageCapture) stopTelegram() {
	if c.telegramCancel != nil {
		c.telegramCancel()
		<-c.telegramDone
		c.telegramCancel = nil
		c.telegramDone = nil
	}
}

func telegramProxy(network messagecapture.NetworkConfiguration) string {
	if network.Source == "" {
		return inbox.TelegramProxyFromEnvironment()
	}
	if !network.Enabled {
		return ""
	}
	return network.ProxyURL
}

func (c *messageCapture) setWeCom(setting messagecapture.WeCom) {
	if !setting.Enabled {
		c.weComCallback.Store(nil)
		return
	}
	agentID, _ := strconv.ParseInt(setting.AgentID, 10, 64)
	handler, err := inbox.NewWeCom(setting.CorpID, agentID, setting.CallbackToken, setting.EncodingAESKey, setting.UserID, func(ctx context.Context, messageID, text string) error {
		return c.saveMemo(ctx, "wechat", messageID, text)
	})
	if err != nil {
		c.weComCallback.Store(nil)
		slog.Warn("WeCom capture disabled: invalid callback configuration")
		return
	}
	c.weComCallback.Store(&handler)
	slog.Info("WeCom capture enabled", slog.String("callbackPath", messagecapture.WeComCallbackPath))
}

func (c *messageCapture) serveWeCom(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	deadline := time.Now().Add(5 * time.Second)
	// Keep the deadline through net/http's response finalization, which
	// can drain the unread body of an early rejection.
	_ = http.NewResponseController(w).SetReadDeadline(deadline)
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	defer cancel()
	callback := c.weComCallback.Load()
	if callback == nil {
		http.NotFound(w, r)
		return
	}
	(*callback).ServeHTTP(w, r.WithContext(ctx))
}
