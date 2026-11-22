package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/core/messagecapture"
	"github.com/usememos/memos/internal/httphandler"
)

const (
	notificationSettingsPath = "/api/notificationSettings"
	networkSettingsPath      = "/api/networkSettings"
	maxCaptureSettingsBytes  = 8 << 10
)

// NotificationSettingsService manages both external inboxes.
type NotificationSettingsService interface {
	GetConfiguration(context.Context) (messagecapture.Configuration, error)
	UpdateConfiguration(context.Context, messagecapture.Update) (messagecapture.Configuration, error)
}

// NetworkSettingsService manages the Telegram network proxy.
type NetworkSettingsService interface {
	GetNetworkConfiguration(context.Context) (messagecapture.NetworkConfiguration, error)
	UpdateNetworkConfiguration(context.Context, messagecapture.NetworkUpdate) (messagecapture.NetworkConfiguration, error)
}

// The HTTP views only expose whether credentials are available. They never
// marshal runtime configuration or a proxy URL that may include credentials.
type telegramSettingsView struct {
	Enabled     bool                  `json:"enabled"`
	Configured  bool                  `json:"configured"`
	Source      messagecapture.Source `json:"source"`
	UserID      string                `json:"userId"`
	HasBotToken bool                  `json:"hasBotToken"`
}

type weComSettingsView struct {
	Enabled           bool                  `json:"enabled"`
	Configured        bool                  `json:"configured"`
	Source            messagecapture.Source `json:"source"`
	CorpID            string                `json:"corpId"`
	AgentID           string                `json:"agentId"`
	UserID            string                `json:"userId"`
	HasCallbackToken  bool                  `json:"hasCallbackToken"`
	HasEncodingAESKey bool                  `json:"hasEncodingAesKey"`
}

type notificationSettingsView struct {
	Telegram          telegramSettingsView `json:"telegram"`
	WeCom             weComSettingsView    `json:"wecom"`
	WeComCallbackPath string               `json:"wecomCallbackPath"`
}

type networkSettingsView struct {
	Enabled     bool                  `json:"enabled"`
	HasProxyURL bool                  `json:"hasProxyUrl"`
	Source      messagecapture.Source `json:"source"`
}

func (s *APIService) newCaptureSettingsHandler(authorizer *Authorizer) http.Handler {
	mux := http.NewServeMux()
	if s.personalMode() && !s.Profile.Demo {
		if s.NotificationSettings != nil {
			for _, method := range []string{http.MethodGet, http.MethodPatch} {
				mux.HandleFunc(method+" "+notificationSettingsPath, s.authorizeAdminSession(authorizer, s.serveNotificationSettings))
			}
		}
		if s.NetworkSettings != nil {
			for _, method := range []string{http.MethodGet, http.MethodPatch} {
				mux.HandleFunc(method+" "+networkSettingsPath, s.authorizeAdminSession(authorizer, s.serveNetworkSettings))
			}
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setAPIResponseNoStoreHeaders(w.Header())
		if rejectsBrowserOrigin(r, s.Profile.InstanceURL) {
			writeGatewayStatusError(w, status.Error(codes.PermissionDenied, "browser origin is not allowed for this settings request"))
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (s *APIService) serveNotificationSettings(w http.ResponseWriter, r *http.Request) error {
	var config messagecapture.Configuration
	var err error
	if r.Method == http.MethodPatch {
		var update messagecapture.Update
		if err := decodeCaptureSettings(w, r, &update); err != nil {
			return err
		}
		config, err = s.NotificationSettings.UpdateConfiguration(r.Context(), update)
	} else {
		config, err = s.NotificationSettings.GetConfiguration(r.Context())
	}
	if err != nil {
		return captureSettingsError(err, "could not load notification settings", r.Method)
	}
	return httphandler.JSON(w, http.StatusOK, notificationSettingsView{
		Telegram: telegramSettingsView{
			Enabled: config.Telegram.Enabled, Configured: config.Telegram.Configured(), Source: config.TelegramSource,
			UserID: config.Telegram.UserID, HasBotToken: config.Telegram.BotToken != "",
		},
		WeCom: weComSettingsView{
			Enabled: config.WeCom.Enabled, Configured: config.WeCom.Configured(), Source: config.WeComSource,
			CorpID: config.WeCom.CorpID, AgentID: config.WeCom.AgentID, UserID: config.WeCom.UserID,
			HasCallbackToken: config.WeCom.CallbackToken != "", HasEncodingAESKey: config.WeCom.EncodingAESKey != "",
		},
		WeComCallbackPath: "/integrations/wecom",
	})
}

func (s *APIService) serveNetworkSettings(w http.ResponseWriter, r *http.Request) error {
	var config messagecapture.NetworkConfiguration
	var err error
	if r.Method == http.MethodPatch {
		var input struct {
			Enabled  *bool  `json:"enabled"`
			ProxyURL string `json:"proxyUrl"`
		}
		if err := decodeCaptureSettings(w, r, &input); err != nil {
			return err
		}
		if input.Enabled == nil {
			return status.Error(codes.InvalidArgument, "an enabled status is required")
		}
		config, err = s.NetworkSettings.UpdateNetworkConfiguration(r.Context(), messagecapture.NetworkUpdate{
			Enabled: *input.Enabled, ProxyURL: input.ProxyURL,
		})
	} else {
		config, err = s.NetworkSettings.GetNetworkConfiguration(r.Context())
	}
	if err != nil {
		return captureSettingsError(err, "could not load network settings", r.Method)
	}
	return httphandler.JSON(w, http.StatusOK, networkSettingsView{
		Enabled: config.Enabled, HasProxyURL: config.ProxyURL != "", Source: config.Source,
	})
}

func decodeCaptureSettings(w http.ResponseWriter, r *http.Request, input any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCaptureSettingsBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(input); err != nil {
		return status.Error(codes.InvalidArgument, "invalid settings request")
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return status.Error(codes.InvalidArgument, "invalid settings request")
	}
	return nil
}

func captureSettingsError(err error, loadMessage, method string) error {
	if errors.Is(err, messagecapture.ErrInvalidConfiguration) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if method == http.MethodPatch {
		return status.Error(codes.Internal, "could not save settings")
	}
	return status.Error(codes.Internal, loadMessage)
}
