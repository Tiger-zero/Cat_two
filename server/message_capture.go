package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/usememos/memos/server/inbox"
)

const weComCallbackPath = "/integrations/wecom"

func (s *Server) configureMessageCapture(mux *http.ServeMux) {
	callback := http.NotFoundHandler()
	if s.Profile.Personal && !s.Profile.Demo {
		s.configureTelegramCapture()
		callback = s.weComCaptureHandler()
	}

	// WeCom authenticates the request with its own signature and encryption.
	// This endpoint receives messages and offers no access to the note API.
	mux.HandleFunc(weComCallbackPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		deadline := time.Now().Add(5 * time.Second)
		controller := http.NewResponseController(w)
		if controller.SetReadDeadline(deadline) == nil {
			defer func() { _ = controller.SetReadDeadline(time.Time{}) }()
		}
		ctx, cancel := context.WithDeadline(r.Context(), deadline)
		defer cancel()
		callback.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) configureTelegramCapture() {
	token := captureEnv("CAT_TWO_TELEGRAM_BOT_TOKEN")
	sender := captureEnv("CAT_TWO_TELEGRAM_USER_ID")
	if token == "" && sender == "" {
		return
	}
	senderID, err := strconv.ParseInt(sender, 10, 64)
	if err != nil || senderID <= 0 || !validTelegramToken(token) {
		slog.Warn("Telegram capture disabled: configure a BotFather token and a positive Telegram user ID")
		return
	}
	botID, _, _ := strings.Cut(token, ":")
	s.receiveTelegram = func(ctx context.Context) {
		inbox.RunTelegram(ctx, token, senderID, func(ctx context.Context, messageID, text string) error {
			return s.apiService.CaptureMemo(ctx, "telegram", botID+"/"+messageID, text)
		})
	}
	slog.Info("Telegram capture enabled")
}

func (s *Server) weComCaptureHandler() http.Handler {
	corpID := captureEnv("CAT_TWO_WECOM_CORP_ID")
	agent := captureEnv("CAT_TWO_WECOM_AGENT_ID")
	token := captureEnv("CAT_TWO_WECOM_CALLBACK_TOKEN")
	aesKey := captureEnv("CAT_TWO_WECOM_ENCODING_AES_KEY")
	sender := captureEnv("CAT_TWO_WECOM_USER_ID")
	if corpID == "" && agent == "" && token == "" && aesKey == "" && sender == "" {
		return http.NotFoundHandler()
	}
	agentID, err := strconv.ParseInt(agent, 10, 64)
	if err != nil || agentID <= 0 || corpID == "" || token == "" || aesKey == "" || sender == "" {
		slog.Warn("WeCom capture disabled: configure the corporate ID, agent ID, callback token, encoding AES key and user ID")
		return http.NotFoundHandler()
	}
	handler, err := inbox.NewWeCom(corpID, agentID, token, aesKey, sender, func(ctx context.Context, messageID, text string) error {
		return s.apiService.CaptureMemo(ctx, "wechat", messageID, text)
	})
	if err != nil {
		slog.Warn("WeCom capture disabled: invalid callback configuration")
		return http.NotFoundHandler()
	}
	slog.Info("WeCom capture enabled", slog.String("callbackPath", weComCallbackPath))
	return handler
}

func captureEnv(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func validTelegramToken(token string) bool {
	bot, secret, ok := strings.Cut(token, ":")
	botID, err := strconv.ParseUint(bot, 10, 64)
	if !ok || err != nil || botID == 0 || secret == "" || len(token) > 128 {
		return false
	}
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
	return !strings.ContainsFunc(secret, func(r rune) bool { return !strings.ContainsRune(alphabet, r) })
}
