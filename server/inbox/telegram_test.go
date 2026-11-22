package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/bot123:synthetic/getUpdates", r.URL.EscapedPath())
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		var request telegramPollRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		assert.Equal(t, []string{"message"}, request.AllowedUpdates)
		assert.Positive(t, request.Timeout)
		switch polls.Add(1) {
		case 1:
			assert.Zero(t, request.Offset)
			_, _ = io.WriteString(w, `{"ok":true,"result":[
				{"update_id":40,"message":{"message_id":8,"from":{"id":123},"chat":{"id":123,"type":"private"},"text":"second"}},
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
			return nil
		}
		saved[id] = text
		if id == secondID {
			// Simulate a write that persisted even though its caller saw an error.
			return errors.New("synthetic write failure")
		}
		return nil
	})
	require.EqualValues(t, 3, polls.Load())
	require.Equal(t, []string{"chat:123:message:7", secondID, secondID, "update:41"}, attempts)
	require.Equal(t, map[string]string{
		"chat:123:message:7": "合成笔记\n- [ ] 测试 #收件箱 ", secondID: "second", "update:41": "fallback",
	}, saved)
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
