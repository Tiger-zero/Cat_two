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
	telegramAPI = "https://api.telegram.org"

	telegramPollTimeout   = 30 // Sent to Telegram in seconds.
	telegramPollLimit     = 50
	telegramResponseLimit = 8 << 20
	telegramMinRetry      = time.Second
	telegramMaxRetry      = 30 * time.Second
	telegramUpdateWindow  = 24 * time.Hour
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

// RunTelegram polls for private messages from senderID until ctx is canceled.
// save receives a stable event ID followed by the original message text.
func RunTelegram(ctx context.Context, token string, senderID int64, save func(context.Context, string, string) error) {
	client := &http.Client{
		Timeout: time.Duration(telegramPollTimeout+10) * time.Second,
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
	endpoint := strings.TrimRight(baseURL, "/") + "/bot" + url.PathEscape(token) + "/getUpdates"
	var offset int64
	var lastUpdate time.Time
	backoff := telegramMinRetry
	for ctx.Err() == nil {
		// Telegram keeps updates for at most a day and may reset their IDs
		// after a week of inactivity. Drop an old offset before that happens.
		if offset != 0 && time.Since(lastUpdate) >= telegramUpdateWindow {
			offset = 0
		}
		updates, err := getTelegramUpdates(ctx, client, endpoint, offset)
		failure := "Telegram polling failed"
		if err == nil {
			if len(updates) > 0 {
				lastUpdate = time.Now()
			}
			offset, err = saveTelegramUpdates(ctx, updates, offset, senderID, save)
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
	payload, err := json.Marshal(telegramPollRequest{
		Offset: offset, Timeout: telegramPollTimeout, Limit: telegramPollLimit, AllowedUpdates: []string{"message"},
	})
	if err != nil {
		return nil, errTelegramPoll
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, errTelegramPoll
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, errTelegramPoll
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errTelegramPoll
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, telegramResponseLimit+1))
	if err != nil || len(data) > telegramResponseLimit {
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

func saveTelegramUpdates(ctx context.Context, updates []telegramUpdate, offset, senderID int64, save func(context.Context, string, string) error) (int64, error) {
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
		}
		// Sending the next offset confirms the updates that precede it. A failed
		// save returns above, which leaves that update eligible for redelivery.
		offset = update.ID + 1
	}
	return offset, nil
}
