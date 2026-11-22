package server

import (
	"bytes"
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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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

	request := func(method, plaintext string) *httptest.ResponseRecorder {
		ciphertext := encryptCaptureFixture(t, key, corp, plaintext)
		parts := []string{token, strconv.FormatInt(time.Now().Unix(), 10), "fixture", ciphertext}
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
		s.handler.ServeHTTP(response, httptest.NewRequest(method, weComCallbackPath+"?"+query.Encode(), strings.NewReader(payload)))
		return response
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
