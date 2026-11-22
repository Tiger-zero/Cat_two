package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTelegramSavesOnlyAuthorizedPrivateTextInOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var polls atomic.Int32
	var replies atomic.Int32
	var completedSaves atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		if r.URL.EscapedPath() == "/bot123:synthetic/sendMessage" {
			count := replies.Add(1)
			assert.Equal(t, count, completedSaves.Load(), "confirmation must follow the successful write")
			var reply telegramReplyRequest
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&reply))
			assert.EqualValues(t, 123, reply.ChatID)
			assert.Equal(t, "Yes!", reply.Text)
			switch count {
			case 1:
				assert.Equal(t, &telegramReplyParameters{MessageID: 7, AllowSendingWithoutReply: true}, reply.ReplyParameters)
			case 2:
				assert.Equal(t, &telegramReplyParameters{MessageID: 8, AllowSendingWithoutReply: true}, reply.ReplyParameters)
			case 3:
				assert.Nil(t, reply.ReplyParameters)
			default:
				t.Error("unexpected Telegram confirmation")
			}
			_, _ = io.WriteString(w, `{"ok":true}`)
			return
		}
		assert.Equal(t, "/bot123:synthetic/getUpdates", r.URL.EscapedPath())
		var request telegramPollRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		assert.Equal(t, []string{"message"}, request.AllowedUpdates)
		assert.Positive(t, request.Timeout)
		switch polls.Add(1) {
		case 1:
			assert.Zero(t, request.Offset)
			_, _ = io.WriteString(w, `{"ok":true,"result":[
				{"update_id":40,"message":{"message_id":8,"from":{"id":123},"chat":{"id":123,"type":"private"},"text":"second"}},
				{"update_id":32,"message":{"message_id":1,"from":{"id":123},"chat":{"id":123,"type":"private"},"text":" \t"}},
				{"update_id":33,"edited_message":{"message_id":1,"from":{"id":123},"chat":{"id":123,"type":"private"},"text":"edit"}},
				{"update_id":34,"message":{"message_id":2,"chat":{"id":123,"type":"private"},"text":"missing sender"}},
				{"update_id":35,"message":{"message_id":3,"from":{"id":123},"chat":{"id":123,"type":"private"},"caption":"caption only"}},
				{"update_id":37,"message":{"message_id":5,"from":{"id":456},"chat":{"id":123,"type":"private"},"text":"another sender"}},
				{"update_id":38,"message":{"message_id":6,"from":{"id":123},"chat":{"id":456,"type":"private"},"text":"another chat"}},
				{"update_id":39,"message":{"message_id":7,"from":{"id":123},"chat":{"id":123,"type":"private"},"text":"合成笔记\n- [ ] 测试 #收件箱 "}},
				{"update_id":36,"message":{"message_id":4,"from":{"id":123},"chat":{"id":123,"type":"group"},"text":"group"}}]}`)
		case 2:
			assert.EqualValues(t, 40, request.Offset, "the failed write must still be eligible for delivery")
			_, _ = io.WriteString(w, `{"ok":true,"result":[
				{"update_id":41,"message":{"from":{"id":123},"chat":{"id":123,"type":"private"},"text":"fallback"}},
				{"update_id":39,"message":{"message_id":7,"from":{"id":123},"chat":{"id":123,"type":"private"},"text":"stale"}},
				{"update_id":40,"message":{"message_id":8,"from":{"id":123},"chat":{"id":123,"type":"private"},"text":"second"}},
				{"update_id":40,"message":{"message_id":8,"from":{"id":123},"chat":{"id":123,"type":"private"},"text":"duplicate"}}]}`)
		case 3:
			assert.EqualValues(t, 42, request.Offset)
			cancel()
		default:
			t.Error("unexpected Telegram poll")
			cancel()
		}
	}))
	t.Cleanup(server.Close)

	var attempts []string
	saved := make(map[string]string)
	const secondID = "chat:123:message:8"
	pollTelegram(ctx, server.Client(), server.URL, "123:synthetic", 123, func(_ context.Context, id, text string) error {
		attempts = append(attempts, id)
		if previous, exists := saved[id]; exists {
			require.Equal(t, previous, text)
			completedSaves.Add(1)
			return nil
		}
		saved[id] = text
		if id == secondID {
			// Simulate a write that persisted even though its caller saw an error.
			return errors.New("synthetic write failure")
		}
		completedSaves.Add(1)
		return nil
	})
	require.EqualValues(t, 3, polls.Load())
	require.EqualValues(t, 3, replies.Load())
	require.Equal(t, []string{"chat:123:message:7", secondID, secondID, "update:41"}, attempts)
	require.Equal(t, map[string]string{
		"chat:123:message:7": "合成笔记\n- [ ] 测试 #收件箱 ", secondID: "second", "update:41": "fallback",
	}, saved)
}

func TestTelegramFailedConfirmationDoesNotRetryNoteOrBlockLaterMessages(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var polls atomic.Int32
	var eventsMu sync.Mutex
	var events []string
	record := func(event string) {
		eventsMu.Lock()
		defer eventsMu.Unlock()
		events = append(events, event)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/bot123:synthetic/getUpdates":
			var request telegramPollRequest
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			if polls.Add(1) == 1 {
				assert.Zero(t, request.Offset)
				_, _ = io.WriteString(w, `{"ok":true,"result":[
					{"update_id":40,"message":{"message_id":7,"from":{"id":123},"chat":{"id":123,"type":"private"},"text":"first"}},
					{"update_id":41,"message":{"message_id":8,"from":{"id":123},"chat":{"id":123,"type":"private"},"text":"second"}}]}`)
				return
			}
			assert.EqualValues(t, 42, request.Offset, "confirmation failure must not stop the offset from advancing")
			record("checkpoint")
			cancel()
		case "/bot123:synthetic/sendMessage":
			var request telegramReplyRequest
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			if request.ReplyParameters != nil && request.ReplyParameters.MessageID == 7 {
				record("first confirmation attempt")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = io.WriteString(w, `{"ok":false,"error_code":503,"description":"synthetic transient error"}`)
				return
			}
			assert.Equal(t, &telegramReplyParameters{MessageID: 8, AllowSendingWithoutReply: true}, request.ReplyParameters)
			record("second confirmation")
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			t.Error("unexpected Telegram request")
			cancel()
		}
	}))
	t.Cleanup(server.Close)

	pollTelegram(ctx, server.Client(), server.URL, "123:synthetic", 123, func(_ context.Context, _, text string) error {
		record("save " + text)
		return nil
	})
	eventsMu.Lock()
	defer eventsMu.Unlock()
	require.Equal(t, []string{"save first", "first confirmation attempt", "first confirmation attempt", "save second", "second confirmation", "checkpoint"}, events)
}

func TestTelegramConfirmationRetriesOnlyWhenDelayFits(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		sent     bool
		attempts int32
	}{
		{name: "short rate limit", status: http.StatusTooManyRequests, body: `{"ok":false,"error_code":429,"parameters":{"retry_after":1}}`, sent: true, attempts: 2},
		{name: "rate limit longer than reply budget", status: http.StatusTooManyRequests, body: `{"ok":false,"error_code":429,"parameters":{"retry_after":60}}`, attempts: 1},
		{name: "ambiguous response", status: http.StatusBadGateway, body: `unavailable`, attempts: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var attempts atomic.Int32
			startedAt := time.Now()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if attempts.Add(1) == 1 {
					w.WriteHeader(tc.status)
					_, _ = io.WriteString(w, tc.body)
					return
				}
				assert.GreaterOrEqual(t, time.Since(startedAt), time.Second)
				_, _ = io.WriteString(w, `{"ok":true}`)
			}))
			t.Cleanup(server.Close)
			sent := sendTelegramConfirmation(t.Context(), server.Client(), server.URL, &telegramMessage{ID: 7})
			require.Equal(t, tc.sent, sent)
			require.Equal(t, tc.attempts, attempts.Load())
		})
	}
}

func TestTelegramPollReturnsSafeErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "API failure body", status: http.StatusUnauthorized, body: `{"ok":false,"description":"synthetic secret"}`},
		{name: "missing result", status: http.StatusOK, body: `{"ok":true}`},
		{name: "negative update ID", status: http.StatusOK, body: `{"ok":true,"result":[{"update_id":-1}]}`},
		{name: "offset overflow", status: http.StatusOK, body: `{"ok":true,"result":[{"update_id":9223372036854775807}]}`},
		{name: "malformed JSON", status: http.StatusOK, body: `{"ok":true,"result":[synthetic secret`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(server.Close)
			updates, err := getTelegramUpdates(t.Context(), server.Client(), server.URL+"/synthetic-secret/getUpdates", 0)
			require.Nil(t, updates)
			require.EqualError(t, err, "telegram polling failed")
		})
	}
}

func TestTelegramCancelsInFlightPoll(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	done := make(chan struct{})
	go func() {
		defer close(done)
		pollTelegram(ctx, server.Client(), server.URL, "123:synthetic", 123, func(context.Context, string, string) error {
			return errors.New("no update expected")
		})
	}()
	select {
	case <-started:
		cancel()
	case <-time.After(3 * time.Second):
		t.Fatal("Telegram poll did not start")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Telegram poll continued after cancellation")
	}
}
