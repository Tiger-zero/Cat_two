package inbox

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
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	weComTestCorp   = "ww-example-corporation"
	weComTestAgent  = 1000042
	weComTestToken  = "synthetic-callback-token"
	weComTestSender = "example-owner"
	weComTestRawKey = "01234567890123456789012345678901"
	weComTestAESKey = "MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE"
)

func TestNewWeComRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, corp, token, aesKey, sender string
		agent                             int64
	}{
		{"blank corporation", "", weComTestToken, weComTestAESKey, weComTestSender, weComTestAgent},
		{"invalid agent", weComTestCorp, weComTestToken, weComTestAESKey, weComTestSender, 0},
		{"blank token", weComTestCorp, "", weComTestAESKey, weComTestSender, weComTestAgent},
		{"invalid AES key length", weComTestCorp, weComTestToken, "short", weComTestSender, weComTestAgent},
		{"invalid AES key encoding", weComTestCorp, weComTestToken, strings.Repeat("!", 43), weComTestSender, weComTestAgent},
		{"invalid sender", weComTestCorp, weComTestToken, weComTestAESKey, "bad/sender", weComTestAgent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewWeCom(tc.corp, tc.agent, tc.token, tc.aesKey, tc.sender, func(context.Context, string, string) error { return nil })
			require.Error(t, err)
		})
	}
	_, err := NewWeCom(weComTestCorp, weComTestAgent, weComTestToken, weComTestAESKey, weComTestSender, nil)
	require.Error(t, err)
}

func TestWeComCallbackVerification(t *testing.T) {
	t.Parallel()
	handler := newWeComTestHandler(t, func(context.Context, string, string) error {
		t.Fatal("GET verification must not save a note")
		return nil
	})
	const echo = "回调验证 <&>\n"
	encrypted := encryptWeComTestMessage(t, echo, weComTestCorp)
	wrongReceiver := encryptWeComTestMessage(t, echo, "ww-other-corporation")
	invalidPadding := bytes.Repeat([]byte{'x'}, 32)
	invalidPadding[31] = 2
	invalidLength := bytes.Repeat([]byte{'x'}, 32)
	invalidLength[31] = 1
	binary.BigEndian.PutUint32(invalidLength[16:], math.MaxUint32)
	for _, tc := range []struct {
		name, ciphertext string
		age              time.Duration
		alterQuery       func(url.Values)
		status           int
	}{
		{name: "verified", ciphertext: encrypted, status: http.StatusOK},
		{name: "invalid signature", ciphertext: encrypted, alterQuery: func(query url.Values) {
			query.Set("msg_signature", strings.Repeat("0", 40))
		}, status: http.StatusForbidden},
		{name: "expired timestamp", ciphertext: encrypted, age: -10 * time.Minute, status: http.StatusForbidden},
		{name: "future timestamp", ciphertext: encrypted, age: 10 * time.Minute, status: http.StatusForbidden},
		{name: "other receiver", ciphertext: wrongReceiver, status: http.StatusForbidden},
		{name: "invalid ciphertext", ciphertext: "invalid-base64!", status: http.StatusForbidden},
		{name: "invalid padding", ciphertext: encryptWeComTestFrame(t, invalidPadding), status: http.StatusForbidden},
		{name: "invalid message length", ciphertext: encryptWeComTestFrame(t, invalidLength), status: http.StatusForbidden},
		{name: "missing echo", ciphertext: encrypted, alterQuery: func(query url.Values) { query.Del("echostr") }, status: http.StatusBadRequest},
		{name: "repeated nonce", ciphertext: encrypted, alterQuery: func(query url.Values) { query.Add("nonce", "another") }, status: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := signWeComTestMessage(tc.ciphertext, time.Now().Add(tc.age))
			query.Set("echostr", tc.ciphertext)
			if tc.alterQuery != nil {
				tc.alterQuery(query)
			}
			response := serveWeComTestRequest(handler, http.MethodGet, query, "")
			require.Equal(t, tc.status, response.Code)
			require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
			if tc.status == http.StatusOK {
				require.Equal(t, "text/plain; charset=utf-8", response.Header().Get("Content-Type"))
				require.Equal(t, echo, response.Body.String())
			} else {
				require.NotContains(t, response.Body.String(), echo)
			}
		})
	}
}

func TestWeComIgnoresUnauthorizedAndNonTextMessages(t *testing.T) {
	t.Parallel()
	handler := newWeComTestHandler(t, func(context.Context, string, string) error {
		t.Fatal("ignored callbacks must not save a note")
		return nil
	})
	for _, tc := range []struct {
		name, messageType, content, sender string
		agent                              int
	}{
		{"other sender", "text", "private note", "someone-else", weComTestAgent},
		{"other agent", "text", "private note", weComTestSender, weComTestAgent + 1},
		{"image", "image", "private note", weComTestSender, weComTestAgent},
		{"blank text", "text", " \n ", weComTestSender, weComTestAgent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message := weComTestMessage(tc.messageType, tc.sender, tc.agent, "23", tc.content)
			ciphertext := encryptWeComTestMessage(t, message, weComTestCorp)
			response := serveWeComTestRequest(handler, http.MethodPost, signWeComTestMessage(ciphertext, time.Now()), wrapWeComTestMessage(ciphertext))
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, "text/plain; charset=utf-8", response.Header().Get("Content-Type"))
			require.Equal(t, "success", response.Body.String())
		})
	}
}

func TestWeComRejectsMalformedMessages(t *testing.T) {
	t.Parallel()
	handler := newWeComTestHandler(t, func(context.Context, string, string) error {
		t.Fatal("invalid callbacks must not save a note")
		return nil
	})
	message := weComTestMessage("text", weComTestSender, weComTestAgent, "23", "private text")
	ciphertext := encryptWeComTestMessage(t, message, weComTestCorp)
	duplicateCiphertext := strings.TrimSuffix(wrapWeComTestMessage(ciphertext), "</xml>") + "<Encrypt>" + ciphertext + "</Encrypt></xml>"
	malformedXML := encryptWeComTestMessage(t, "<xml><MsgType>text", weComTestCorp)
	missingID := encryptWeComTestMessage(t, weComTestMessage("text", weComTestSender, weComTestAgent, "", "private text"), weComTestCorp)
	for _, tc := range []struct {
		name, signedCiphertext, body string
		status                       int
	}{
		{"malformed envelope", ciphertext, "<xml><Encrypt>", http.StatusBadRequest},
		{"missing ciphertext", ciphertext, "<xml></xml>", http.StatusBadRequest},
		{"duplicate ciphertext", ciphertext, duplicateCiphertext, http.StatusBadRequest},
		{"extra root element", ciphertext, wrapWeComTestMessage(ciphertext) + "<xml/>", http.StatusBadRequest},
		{"malformed encrypted XML", malformedXML, wrapWeComTestMessage(malformedXML), http.StatusBadRequest},
		{"missing message ID", missingID, wrapWeComTestMessage(missingID), http.StatusBadRequest},
		{"signature for another body", malformedXML, wrapWeComTestMessage(ciphertext), http.StatusForbidden},
		{"oversized envelope", ciphertext, strings.Repeat("x", 65_537), http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := serveWeComTestRequest(handler, http.MethodPost, signWeComTestMessage(tc.signedCiphertext, time.Now()), tc.body)
			require.Equal(t, tc.status, response.Code)
		})
	}

	response := serveWeComTestRequest(handler, http.MethodPut, nil, "")
	require.Equal(t, http.StatusMethodNotAllowed, response.Code)
	require.Equal(t, "GET, POST", response.Header().Get("Allow"))
}

func TestWeComRetriesWithStableMessageID(t *testing.T) {
	t.Parallel()
	const declaration = `<?xml version="1.0" encoding="UTF-8"?>`
	const expectedText = "  #收集 & <记录>\n下一行  "
	const expectedID = "ww-example-corporation/1000042/example-owner/18446744073709551615"
	persisted := make(map[string]string)
	attempts := 0
	handler := newWeComTestHandler(t, func(_ context.Context, externalID, content string) error {
		attempts++
		require.Equal(t, expectedID, externalID)
		require.Equal(t, expectedText, content)
		if attempts == 1 {
			return errors.New("internal storage details")
		}
		if _, saved := persisted[externalID]; !saved {
			persisted[externalID] = content
		}
		return nil
	})
	message := declaration + weComTestMessage("text", weComTestSender, weComTestAgent, "18446744073709551615", expectedText)
	ciphertext := encryptWeComTestMessage(t, message, weComTestCorp)
	query := signWeComTestMessage(ciphertext, time.Now())
	body := wrapWeComTestMessage(ciphertext)
	failed := serveWeComTestRequest(handler, http.MethodPost, query, body)
	require.Equal(t, http.StatusServiceUnavailable, failed.Code)
	require.Equal(t, "text/plain; charset=utf-8", failed.Header().Get("Content-Type"))
	require.NotContains(t, failed.Body.String(), "internal storage details")
	require.Empty(t, persisted)
	var previousCiphertext string
	for _, envelope := range []string{body, declaration + body} {
		response := serveWeComTestRequest(handler, http.MethodPost, query, envelope)
		ciphertext := requireWeComTestConfirmation(t, response, query.Get("nonce"))
		require.NotEqual(t, previousCiphertext, ciphertext)
		previousCiphertext = ciphertext
	}
	require.Equal(t, 3, attempts)
	require.Equal(t, map[string]string{expectedID: expectedText}, persisted)
}

func requireWeComTestConfirmation(t *testing.T, response *httptest.ResponseRecorder, nonce string) string {
	t.Helper()
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "application/xml; charset=utf-8", response.Header().Get("Content-Type"))
	var envelope struct {
		XMLName   xml.Name `xml:"xml"`
		Encrypt   string   `xml:"Encrypt"`
		Signature string   `xml:"MsgSignature"`
		Timestamp int64    `xml:"TimeStamp"`
		Nonce     string   `xml:"Nonce"`
	}
	require.NoError(t, xml.Unmarshal(response.Body.Bytes(), &envelope))
	require.Equal(t, nonce, envelope.Nonce)
	require.WithinDuration(t, time.Now(), time.Unix(envelope.Timestamp, 0), time.Minute)
	require.Equal(t, signWeComTestMessage(envelope.Encrypt, time.Unix(envelope.Timestamp, 0)).Get("msg_signature"), envelope.Signature)

	frame, err := base64.StdEncoding.DecodeString(envelope.Encrypt)
	require.NoError(t, err)
	require.NotEmpty(t, frame)
	require.Zero(t, len(frame)%32)
	block, err := aes.NewCipher([]byte(weComTestRawKey))
	require.NoError(t, err)
	cipher.NewCBCDecrypter(block, []byte(weComTestRawKey[:aes.BlockSize])).CryptBlocks(frame, frame)
	padding := int(frame[len(frame)-1])
	require.Positive(t, padding)
	require.LessOrEqual(t, padding, 32)
	require.Equal(t, bytes.Repeat([]byte{byte(padding)}, padding), frame[len(frame)-padding:])
	data := frame[:len(frame)-padding]
	require.GreaterOrEqual(t, len(data), 20+len(weComTestCorp))
	size := int(binary.BigEndian.Uint32(data[16:20]))
	require.Equal(t, len(data)-20-len(weComTestCorp), size)
	require.Equal(t, weComTestCorp, string(data[20+size:]))

	var reply struct {
		XMLName    xml.Name `xml:"xml"`
		Recipient  string   `xml:"ToUserName"`
		Sender     string   `xml:"FromUserName"`
		CreateTime int64    `xml:"CreateTime"`
		Type       string   `xml:"MsgType"`
		Content    string   `xml:"Content"`
		AgentID    int64    `xml:"AgentID"`
	}
	require.NoError(t, xml.Unmarshal(data[20:20+size], &reply))
	require.Equal(t, weComTestSender, reply.Recipient)
	require.Equal(t, weComTestCorp, reply.Sender)
	require.Equal(t, envelope.Timestamp, reply.CreateTime)
	require.Equal(t, "text", reply.Type)
	require.Equal(t, "Yes!", reply.Content)
	require.Equal(t, int64(weComTestAgent), reply.AgentID)
	return envelope.Encrypt
}

func newWeComTestHandler(t *testing.T, save func(context.Context, string, string) error) http.Handler {
	t.Helper()
	handler, err := NewWeCom(weComTestCorp, weComTestAgent, weComTestToken, weComTestAESKey, weComTestSender, save)
	require.NoError(t, err)
	return handler
}

// These synthetic fixtures follow WeCom's client-side protocol with a fixed random prefix.
func encryptWeComTestMessage(t *testing.T, message, receiver string) string {
	t.Helper()
	frame := binary.BigEndian.AppendUint32([]byte("test-rand-prefix"), uint32(len(message)))
	frame = append(frame, message...)
	frame = append(frame, receiver...)
	padding := 32 - len(frame)%32
	frame = append(frame, bytes.Repeat([]byte{byte(padding)}, padding)...)
	return encryptWeComTestFrame(t, frame)
}

func encryptWeComTestFrame(t *testing.T, frame []byte) string {
	t.Helper()
	block, err := aes.NewCipher([]byte(weComTestRawKey))
	require.NoError(t, err)
	ciphertext := make([]byte, len(frame))
	cipher.NewCBCEncrypter(block, []byte(weComTestRawKey[:aes.BlockSize])).CryptBlocks(ciphertext, frame)
	return base64.StdEncoding.EncodeToString(ciphertext)
}

func signWeComTestMessage(ciphertext string, when time.Time) url.Values {
	timestamp := strconv.FormatInt(when.Unix(), 10)
	const nonce = "834927"
	parts := []string{nonce, ciphertext, timestamp, weComTestToken}
	slices.Sort(parts)
	signature := sha1.Sum([]byte(strings.Join(parts, "")))
	return url.Values{"timestamp": {timestamp}, "nonce": {nonce}, "msg_signature": {hex.EncodeToString(signature[:])}}
}

func weComTestMessage(messageType, sender string, agent int, id, content string) string {
	return fmt.Sprintf(`<xml><ToUserName><![CDATA[%s]]></ToUserName><FromUserName><![CDATA[%s]]></FromUserName>`+
		`<CreateTime>1670000000</CreateTime><MsgType><![CDATA[%s]]></MsgType><Content><![CDATA[%s]]></Content>`+
		`<MsgId>%s</MsgId><AgentID>%d</AgentID></xml>`, weComTestCorp, sender, messageType, content, id, agent)
}

func wrapWeComTestMessage(ciphertext string) string {
	return "<xml><ToUserName><![CDATA[" + weComTestCorp + "]]></ToUserName><Encrypt><![CDATA[" + ciphertext + "]]></Encrypt><AgentID>1</AgentID></xml>"
}

func serveWeComTestRequest(handler http.Handler, method string, query url.Values, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/integrations/wecom?"+query.Encode(), strings.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
