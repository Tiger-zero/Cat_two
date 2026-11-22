package inbox

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	telegramAPIHost = "api.telegram.org"
	telegramAPI     = "https://" + telegramAPIHost

	telegramPollTimeout   = 30 // Sent to Telegram in seconds.
	telegramPollLimit     = 50
	telegramResponseLimit = 8 << 20
	telegramMinRetry      = time.Second
	telegramMaxRetry      = 30 * time.Second
	telegramUpdateWindow  = 24 * time.Hour
	telegramReplyTimeout  = 4 * time.Second
	telegramReplyRetry    = 250 * time.Millisecond
)

var errTelegramPoll = errors.New("telegram polling failed")

type telegramPollRequest struct {
	Offset         int64    `json:"offset,omitempty"`
	Timeout        int      `json:"timeout"`
	Limit          int      `json:"limit"`
	AllowedUpdates []string `json:"allowed_updates"`
}

type telegramUpdate struct {
	ID      int64            `json:"update_id"`
	Message *telegramMessage `json:"message"`
}

type telegramMessage struct {
	ID   int64  `json:"message_id"`
	Text string `json:"text"`
	From struct {
		ID int64 `json:"id"`
	} `json:"from"`
	Chat struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
}

type telegramReplyRequest struct {
	ChatID          int64                    `json:"chat_id"`
	Text            string                   `json:"text"`
	ReplyParameters *telegramReplyParameters `json:"reply_parameters,omitempty"`
}

type telegramReplyParameters struct {
	MessageID                int64 `json:"message_id"`
	AllowSendingWithoutReply bool  `json:"allow_sending_without_reply"`
}

// TelegramProxyFromEnvironment follows Go's HTTPS_PROXY and NO_PROXY rules
// for Telegram's fixed API origin.
func TelegramProxyFromEnvironment() string {
	proxy, _ := http.ProxyFromEnvironment(&http.Request{URL: &url.URL{Scheme: "https", Host: telegramAPIHost}})
	if proxy == nil {
		return ""
	}
	return proxy.String()
}

// RunTelegram polls for private messages from senderID until ctx is canceled.
// An empty proxyURL connects directly. save receives a stable event ID followed
// by the original message text.
func RunTelegram(ctx context.Context, token string, senderID int64, proxyURL string, save func(context.Context, string, string) error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	transport.Proxy = nil
	if proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			slog.WarnContext(ctx, "Telegram polling disabled by invalid proxy configuration")
			return
		}
		transport.Proxy = http.ProxyURL(proxy)
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   time.Duration(telegramPollTimeout+10) * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	pollTelegram(ctx, client, telegramAPI, token, senderID, save)
}

func pollTelegram(ctx context.Context, client *http.Client, baseURL, token string, senderID int64, save func(context.Context, string, string) error) {
	if token == "" || senderID <= 0 || save == nil {
		return
	}
	botEndpoint := strings.TrimRight(baseURL, "/") + "/bot" + url.PathEscape(token)
	confirm := func(ctx context.Context, message *telegramMessage) {
		if !sendTelegramConfirmation(ctx, client, botEndpoint+"/sendMessage", message) && ctx.Err() == nil {
			slog.WarnContext(ctx, "Telegram note confirmation failed")
		}
	}
	var offset int64
	var lastUpdate time.Time
	backoff := telegramMinRetry
	for ctx.Err() == nil {
		// Telegram keeps updates for at most a day and may reset their IDs
		// after a week of inactivity. Drop an old offset before that happens.
		if offset != 0 && time.Since(lastUpdate) >= telegramUpdateWindow {
			offset = 0
		}
		updates, err := getTelegramUpdates(ctx, client, botEndpoint+"/getUpdates", offset)
		failure := "Telegram polling failed"
		if err == nil {
			if len(updates) > 0 {
				lastUpdate = time.Now()
			}
			offset, err = saveTelegramUpdates(ctx, updates, offset, senderID, save, confirm)
			failure = "Telegram inbox write failed"
		}
		if err == nil {
			backoff = telegramMinRetry
			continue
		}
		if ctx.Err() != nil {
			return
		}
		// The request URL contains the token. The response and sink errors may
		// contain credentials or message text, so log only a static diagnostic.
		slog.WarnContext(ctx, failure, "retry_in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
			backoff = min(backoff*2, telegramMaxRetry)
		}
	}
}

func getTelegramUpdates(ctx context.Context, client *http.Client, endpoint string, offset int64) ([]telegramUpdate, error) {
	status, data := postTelegram(ctx, client, endpoint, telegramPollRequest{
		Offset: offset, Timeout: telegramPollTimeout, Limit: telegramPollLimit, AllowedUpdates: []string{"message"},
	})
	if status != http.StatusOK {
		return nil, errTelegramPoll
	}
	var body struct {
		OK     bool             `json:"ok"`
		Result []telegramUpdate `json:"result"`
	}
	if err := json.Unmarshal(data, &body); err != nil || !body.OK || body.Result == nil || len(body.Result) > telegramPollLimit {
		return nil, errTelegramPoll
	}
	for _, update := range body.Result {
		if update.ID <= 0 || update.ID == math.MaxInt64 {
			return nil, errTelegramPoll
		}
	}
	slices.SortStableFunc(body.Result, func(a, b telegramUpdate) int { return cmp.Compare(a.ID, b.ID) })
	return body.Result, nil
}

func postTelegram(ctx context.Context, client *http.Client, endpoint string, payload any) (int, []byte) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return 0, nil
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return 0, nil
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, telegramResponseLimit+1))
	if err != nil || len(data) > telegramResponseLimit {
		return 0, nil
	}
	return response.StatusCode, data
}

func sendTelegramConfirmation(ctx context.Context, client *http.Client, endpoint string, message *telegramMessage) bool {
	ctx, cancel := context.WithTimeout(ctx, telegramReplyTimeout)
	defer cancel()
	request := telegramReplyRequest{ChatID: message.Chat.ID, Text: "Yes!"}
	if message.ID > 0 {
		request.ReplyParameters = &telegramReplyParameters{MessageID: message.ID, AllowSendingWithoutReply: true}
	}

	sent, retry := postTelegramConfirmation(ctx, client, endpoint, request)
	deadline, _ := ctx.Deadline()
	if sent || retry <= 0 || time.Until(deadline) <= retry {
		return sent
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(retry):
		sent, _ := postTelegramConfirmation(ctx, client, endpoint, request)
		return sent
	}
}

func postTelegramConfirmation(ctx context.Context, client *http.Client, endpoint string, request telegramReplyRequest) (bool, time.Duration) {
	status, data := postTelegram(ctx, client, endpoint, request)
	var body struct {
		OK         *bool `json:"ok"`
		ErrorCode  int   `json:"error_code"`
		Parameters struct {
			RetryAfter int64 `json:"retry_after"`
		} `json:"parameters"`
	}
	// Retry only an explicit rejection. A transport failure can mean Telegram
	// sent the reply while the client lost the response.
	if err := json.Unmarshal(data, &body); err != nil || body.OK == nil {
		return false, 0
	}
	if *body.OK {
		return status == http.StatusOK, 0
	}
	if body.ErrorCode == http.StatusTooManyRequests || status == http.StatusTooManyRequests {
		if body.Parameters.RetryAfter >= int64(telegramReplyTimeout/time.Second) {
			return false, 0
		}
		return false, max(telegramReplyRetry, time.Duration(max(0, body.Parameters.RetryAfter))*time.Second)
	}
	if status >= http.StatusInternalServerError && status < 600 {
		return false, telegramReplyRetry
	}
	return false, 0
}

func saveTelegramUpdates(ctx context.Context, updates []telegramUpdate, offset, senderID int64, save func(context.Context, string, string) error, confirm func(context.Context, *telegramMessage)) (int64, error) {
	for _, update := range updates {
		if err := ctx.Err(); err != nil {
			return offset, err
		}
		if update.ID < offset {
			continue
		}
		message := update.Message
		if message != nil && message.Chat.Type == "private" && message.Chat.ID == senderID && message.From.ID == senderID && strings.TrimSpace(message.Text) != "" {
			eventID := fmt.Sprintf("update:%d", update.ID)
			if message.ID > 0 {
				eventID = fmt.Sprintf("chat:%d:message:%d", message.Chat.ID, message.ID)
			}
			if err := save(ctx, eventID, message.Text); err != nil {
				return offset, err
			}
			confirm(ctx, message)
		}
		// Sending the next offset confirms the updates that precede it. A failed
		// save returns above, which leaves that update eligible for redelivery.
		offset = update.ID + 1
	}
	return offset, nil
}
