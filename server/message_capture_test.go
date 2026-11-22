package server

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/core/messagecapture"
	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/store"
	teststore "github.com/usememos/memos/store/test"
)

func TestWeComMessageCapture(t *testing.T) {
	ctx := t.Context()
	db := teststore.NewTestingStore(ctx, t)
	owner, err := db.CreateUser(ctx, &store.User{Username: "capture-owner", Role: store.RoleAdmin})
	require.NoError(t, err)

	t.Setenv("CAT_TWO_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("CAT_TWO_TELEGRAM_USER_ID", "")
	const corp = "ww-fixture"
	const token = "testcallbacktoken"
	key := bytes.Repeat([]byte{0x42}, 32)
	t.Setenv("CAT_TWO_WECOM_CORP_ID", corp)
	t.Setenv("CAT_TWO_WECOM_AGENT_ID", "1000002")
	t.Setenv("CAT_TWO_WECOM_CALLBACK_TOKEN", token)
	t.Setenv("CAT_TWO_WECOM_ENCODING_AES_KEY", base64.RawStdEncoding.EncodeToString(key))
	t.Setenv("CAT_TWO_WECOM_USER_ID", "allowed")
	s, err := NewServer(ctx, &profile.Profile{Personal: true, Data: t.TempDir()}, db)
	require.NoError(t, err)
	t.Cleanup(func() { s.apiService.CloseUploads(); s.apiService.SSEHub.Close() })

	requestWithToken := func(method, plaintext, callbackToken string) *httptest.ResponseRecorder {
		ciphertext := encryptCaptureFixture(t, key, corp, plaintext)
		parts := []string{callbackToken, strconv.FormatInt(time.Now().Unix(), 10), "fixture", ciphertext}
		query := url.Values{"timestamp": {parts[1]}, "nonce": {parts[2]}}
		slices.Sort(parts)
		digest := sha1.Sum([]byte(strings.Join(parts, "")))
		query.Set("msg_signature", hex.EncodeToString(digest[:]))
		payload := ""
		if method == http.MethodGet {
			query.Set("echostr", ciphertext)
		} else {
			payload = "<xml><Encrypt>" + ciphertext + "</Encrypt></xml>"
		}
		response := httptest.NewRecorder()
		writer := &captureDeadlineWriter{ResponseRecorder: response}
		s.handler.ServeHTTP(writer, httptest.NewRequest(method, messagecapture.WeComCallbackPath+"?"+query.Encode(), strings.NewReader(payload)))
		require.False(t, writer.readDeadline.IsZero(), "read deadline must cover final response handling")
		return response
	}
	request := func(method, plaintext string) *httptest.ResponseRecorder {
		return requestWithToken(method, plaintext, token)
	}
	verification := request(http.MethodGet, "fixture-challenge")
	require.Equal(t, http.StatusOK, verification.Code)
	require.Equal(t, "fixture-challenge", verification.Body.String())
	require.Equal(t, "no-store", verification.Header().Get("Cache-Control"))

	message := `<xml><ToUserName>ww-fixture</ToUserName><FromUserName>allowed</FromUserName><MsgType>text</MsgType><Content>收到笔记</Content><MsgId>760020001</MsgId><AgentID>1000002</AgentID></xml>`
	ignored := request(http.MethodPost, strings.Replace(message, ">allowed<", ">denied<", 1))
	require.Equal(t, http.StatusOK, ignored.Code)
	require.Equal(t, "success", ignored.Body.String())
	for range 2 {
		response := request(http.MethodPost, message)
		require.Equal(t, http.StatusOK, response.Code)
		require.Equal(t, "application/xml; charset=utf-8", response.Header().Get("Content-Type"))
		require.NotContains(t, response.Body.String(), "Yes!")
		var envelope struct {
			XMLName xml.Name `xml:"xml"`
			Encrypt string
		}
		require.NoError(t, xml.Unmarshal(response.Body.Bytes(), &envelope))
		var reply struct {
			ToUserName   string
			FromUserName string
			MsgType      string
			Content      string
		}
		require.NoError(t, xml.Unmarshal(decryptCaptureFixture(t, key, envelope.Encrypt), &reply))
		require.Equal(t, "allowed", reply.ToUserName)
		require.Equal(t, corp, reply.FromUserName)
		require.Equal(t, "text", reply.MsgType)
		require.Equal(t, "Yes!", reply.Content)
	}
	memos, err := db.ListMemos(ctx, &store.FindMemo{})
	require.NoError(t, err)
	require.Len(t, memos, 1)
	require.Equal(t, owner.ID, memos[0].CreatorID)
	require.Equal(t, store.Private, memos[0].Visibility)
	require.Equal(t, "#微信\n\n收到笔记", memos[0].Content)
	require.Equal(t, "wechat", memos[0].Payload.GetCreationChannel())
	require.Contains(t, memos[0].Payload.GetTags(), "微信")

	capture := s.messageCapture
	settings, err := capture.GetConfiguration(ctx)
	require.NoError(t, err)
	require.Equal(t, messagecapture.SourceEnvironment, settings.WeComSource)
	rotated := messagecapture.WeComUpdate{
		Enabled: true, CorpID: corp, AgentID: "1000002", UserID: "allowed",
		CallbackToken: "rotated-callback-token",
	}
	settings, err = capture.UpdateConfiguration(ctx, messagecapture.Update{WeCom: &rotated})
	require.NoError(t, err)
	require.Equal(t, messagecapture.SourceSettings, settings.WeComSource)
	require.Equal(t, base64.RawStdEncoding.EncodeToString(key), settings.WeCom.EncodingAESKey)
	require.Equal(t, http.StatusForbidden, request(http.MethodGet, "old-challenge").Code)
	require.Equal(t, http.StatusOK, requestWithToken(http.MethodGet, "new-challenge", rotated.CallbackToken).Code)

	rotated.Enabled = false
	_, err = capture.UpdateConfiguration(ctx, messagecapture.Update{WeCom: &rotated})
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, requestWithToken(http.MethodGet, "disabled", rotated.CallbackToken).Code)
}

func TestTelegramCaptureReloadsChangedSettings(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	db := teststore.NewTestingStore(ctx, t)
	const initialToken = "42:fixture"
	t.Setenv("CAT_TWO_TELEGRAM_BOT_TOKEN", initialToken)
	t.Setenv("CAT_TWO_TELEGRAM_USER_ID", "1024")
	s, err := NewServer(ctx, &profile.Profile{Personal: true, Data: t.TempDir()}, db)
	require.NoError(t, err)
	t.Cleanup(func() { s.apiService.CloseUploads(); s.apiService.SSEHub.Close() })

	type poller struct {
		token  string
		sender int64
		proxy  string
		done   <-chan struct{}
	}
	started := make(chan poller, 4)
	var active atomic.Int32
	var overlap atomic.Bool
	capture := s.messageCapture
	capture.runTelegram = func(ctx context.Context, token string, sender int64, proxy string, _ func(context.Context, string, string) error) {
		if active.Add(1) != 1 {
			overlap.Store(true)
		}
		defer active.Add(-1)
		started <- poller{token: token, sender: sender, proxy: proxy, done: ctx.Done()}
		<-ctx.Done()
	}
	capture.start(ctx)
	t.Cleanup(capture.stop)

	waitForPoller := func() poller {
		t.Helper()
		select {
		case run := <-started:
			return run
		case <-time.After(3 * time.Second):
			t.Fatal("Telegram poller did not start")
			return poller{}
		}
	}
	expectRunning := func(run poller) {
		t.Helper()
		select {
		case <-run.done:
			t.Fatal("Telegram poller restarted unexpectedly")
		default:
		}
	}

	initial := waitForPoller()
	require.Equal(t, initialToken, initial.token)
	require.EqualValues(t, 1024, initial.sender)
	_, err = capture.UpdateConfiguration(ctx, messagecapture.Update{WeCom: &messagecapture.WeComUpdate{}})
	require.NoError(t, err)
	expectRunning(initial)

	const proxy = "http://proxy.example.test:3128"
	_, err = capture.UpdateNetworkConfiguration(ctx, messagecapture.NetworkUpdate{Enabled: true, ProxyURL: proxy})
	require.NoError(t, err)
	proxied := waitForPoller()
	require.Equal(t, proxy, proxied.proxy)
	_, err = capture.UpdateNetworkConfiguration(ctx, messagecapture.NetworkUpdate{Enabled: true})
	require.NoError(t, err)
	expectRunning(proxied)

	settings, err := capture.UpdateConfiguration(ctx, messagecapture.Update{
		Telegram: &messagecapture.Telegram{Enabled: true, UserID: "2048"},
	})
	require.NoError(t, err)
	require.Equal(t, messagecapture.SourceSettings, settings.TelegramSource)
	updated := waitForPoller()
	require.Equal(t, initialToken, updated.token, "omitting the token retains the effective credential")
	require.EqualValues(t, 2048, updated.sender)
	require.Equal(t, proxy, updated.proxy)

	_, err = capture.UpdateNetworkConfiguration(ctx, messagecapture.NetworkUpdate{Enabled: false})
	require.NoError(t, err)
	direct := waitForPoller()
	require.Empty(t, direct.proxy, "an explicit disable connects directly even with host proxy variables")

	saveErrors := make(chan error, 2)
	go func() {
		_, err := capture.UpdateConfiguration(ctx, messagecapture.Update{
			Telegram: &messagecapture.Telegram{Enabled: true, UserID: "4096"},
		})
		saveErrors <- err
	}()
	go func() {
		_, err := capture.UpdateNetworkConfiguration(ctx, messagecapture.NetworkUpdate{Enabled: true})
		saveErrors <- err
	}()
	for range 2 {
		require.NoError(t, <-saveErrors)
	}
	_ = waitForPoller()
	concurrent := waitForPoller()
	require.EqualValues(t, 4096, concurrent.sender)
	require.Equal(t, proxy, concurrent.proxy)
	cancel()
	capture.stop()
	require.Zero(t, active.Load())

	// Disabling after the worker has stopped still saves its override.
	_, err = capture.UpdateConfiguration(t.Context(), messagecapture.Update{
		Telegram: &messagecapture.Telegram{UserID: "4096"},
	})
	require.NoError(t, err)
	require.Zero(t, active.Load())
	require.False(t, overlap.Load(), "the previous poller must exit before another starts")

	reloaded, err := capture.repository.Load(t.Context(), capture.fallback)
	require.NoError(t, err)
	require.Equal(t, messagecapture.SourceSettings, reloaded.TelegramSource)
	require.False(t, reloaded.Telegram.Enabled, "disabling persists across restart despite the legacy environment")
}

type captureDeadlineWriter struct {
	*httptest.ResponseRecorder
	readDeadline time.Time
}

func (w *captureDeadlineWriter) SetReadDeadline(deadline time.Time) error {
	w.readDeadline = deadline
	return nil
}

func encryptCaptureFixture(t *testing.T, key []byte, corp, text string) string {
	t.Helper()
	data := make([]byte, 20)
	binary.BigEndian.PutUint32(data[16:], uint32(len(text)))
	data = append(data, text...)
	data = append(data, corp...)
	pad := byte(32 - len(data)%32)
	data = append(data, bytes.Repeat([]byte{pad}, int(pad))...)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	cipher.NewCBCEncrypter(block, key[:aes.BlockSize]).CryptBlocks(data, data)
	return base64.StdEncoding.EncodeToString(data)
}

func decryptCaptureFixture(t *testing.T, key []byte, ciphertext string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(data), 32)
	require.Zero(t, len(data)%aes.BlockSize)
	block, err := aes.NewCipher(key)
	require.NoError(t, err)
	cipher.NewCBCDecrypter(block, key[:aes.BlockSize]).CryptBlocks(data, data)
	length := int(binary.BigEndian.Uint32(data[16:20]))
	require.LessOrEqual(t, length, len(data)-20)
	return data[20 : 20+length]
}
