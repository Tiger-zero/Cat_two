package test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPersonalCaptureSettings(t *testing.T) {
	const (
		notificationPath = "/api/notificationSettings"
		networkPath      = "/api/networkSettings"
		callbackPath     = "/integrations/wecom"
		publicCallback   = "https://notes.example.test:8443" + callbackPath
		defaultCallback  = "https://fallback.example.test:9443/notes" + callbackPath
		environmentToken = "test-environment-callback-token"
		callbackToken    = "test-rotated-callback-token"
		proxyURL         = "socks5h://fixture-user:fixture-password@127.0.0.1:1080"
	)
	encodingAESKey := base64.RawStdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	t.Setenv("CAT_TWO_TELEGRAM_USER_ID", "")
	t.Setenv("CAT_TWO_TELEGRAM_BOT_TOKEN", "")
	t.Setenv("CAT_TWO_WECOM_CORP_ID", "wwfixture")
	t.Setenv("CAT_TWO_WECOM_AGENT_ID", "1000001")
	t.Setenv("CAT_TWO_WECOM_USER_ID", "fixture")
	t.Setenv("CAT_TWO_WECOM_CALLBACK_TOKEN", environmentToken)
	t.Setenv("CAT_TWO_WECOM_ENCODING_AES_KEY", encodingAESKey)

	opts := instanceOptions{personal: true, dataDir: t.TempDir(), instanceURL: "http://192.168.5.3:5230"}
	inst := bootInstance(t.Context(), t, opts)
	inst.createAdmin(t)
	admin := inst.signIn(t)
	code, body := inst.do(t, http.MethodPost, "/api/users/"+testAdminUsername+"/personalAccessTokens", admin, map[string]string{"description": "external client fixture"})
	require.Equal(t, http.StatusOK, code)
	var pat struct{ Token string }
	require.NoError(t, json.Unmarshal(body, &pat))
	require.NotEmpty(t, pat.Token)
	for _, path := range []string{notificationPath, networkPath} {
		for _, method := range []string{http.MethodGet, http.MethodPatch} {
			code, _ = inst.do(t, method, path, "", nil)
			require.Equal(t, http.StatusUnauthorized, code)
			code, _ = inst.do(t, method, path, pat.Token, nil)
			require.Equal(t, http.StatusForbidden, code)
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, inst.baseURL+path, nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+admin)
		req.Header.Set("Origin", "https://unrelated.example.test")
		resp, err := inst.client.Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusForbidden, resp.StatusCode)
		require.Contains(t, resp.Header.Get("Cache-Control"), "no-store")
	}

	code, body = inst.do(t, http.MethodGet, notificationPath, admin, nil)
	require.Equal(t, http.StatusOK, code)
	require.JSONEq(t, `{"telegram":{"enabled":false,"configured":false,"source":"","userId":"","hasBotToken":false},
		"wecom":{"enabled":true,"configured":true,"source":"environment","corpId":"wwfixture","agentId":"1000001",
		"userId":"fixture","hasCallbackToken":true,"hasEncodingAesKey":true,"callbackUrl":""},"wecomCallbackPath":"/integrations/wecom"}`, string(body))

	code, _ = inst.do(t, http.MethodPatch, notificationPath, admin, map[string]any{"telegram": map[string]bool{}, "wecom": map[string]bool{}})
	require.Equal(t, http.StatusBadRequest, code)
	code, _ = inst.do(t, http.MethodPatch, notificationPath, admin, map[string]any{"telegram": map[string]bool{"unexpected": true}})
	require.Equal(t, http.StatusBadRequest, code)
	code, body = inst.do(t, http.MethodPatch, notificationPath, admin, map[string]any{"wecom": map[string]any{
		"enabled": true, "corpId": "wwfixture", "agentId": "1000001", "userId": "fixture", "callbackToken": callbackToken,
		"callbackUrl": "notes.example.test:8443",
	}})
	require.Equal(t, http.StatusOK, code, "%s", body)
	require.Contains(t, string(body), `"source":"settings"`)
	require.Contains(t, string(body), `"hasEncodingAesKey":true`)
	require.Contains(t, string(body), `"callbackUrl":"`+publicCallback+`"`)
	require.NotContains(t, string(body), callbackToken)
	require.NotContains(t, string(body), encodingAESKey)
	code, _ = inst.do(t, http.MethodPatch, notificationPath, admin, map[string]any{"wecom": map[string]any{
		"enabled": false, "callbackUrl": "https://192.168.5.3:5230",
	}})
	require.Equal(t, http.StatusBadRequest, code, "an invalid address must not turn off an active callback")

	// An unsigned probe reaches the callback only while the channel is active.
	code, _ = inst.do(t, http.MethodGet, callbackPath, "", nil)
	require.Equal(t, http.StatusBadRequest, code)
	code, body = inst.do(t, http.MethodPatch, notificationPath, admin, map[string]any{"wecom": map[string]any{
		"enabled": false, "corpId": "wwfixture", "agentId": "1000001", "userId": "fixture",
	}})
	require.Equal(t, http.StatusOK, code, "%s", body)
	require.Contains(t, string(body), `"callbackUrl":"`+publicCallback+`"`, "clients that omit the field must keep the saved address")
	code, _ = inst.do(t, http.MethodGet, callbackPath, "", nil)
	require.Equal(t, http.StatusNotFound, code)

	code, body = inst.do(t, http.MethodGet, networkPath, admin, nil)
	require.Equal(t, http.StatusOK, code)
	require.JSONEq(t, `{"enabled":false,"hasProxyUrl":false,"source":""}`, string(body))
	code, body = inst.do(t, http.MethodPatch, networkPath, admin, map[string]any{"enabled": true, "proxyUrl": "ftp://fixture-user:fixture-password@localhost"})
	require.Equal(t, http.StatusBadRequest, code)
	require.NotContains(t, string(body), "fixture-password")
	code, _ = inst.do(t, http.MethodPatch, networkPath, admin, map[string]string{"proxyUrl": proxyURL})
	require.Equal(t, http.StatusBadRequest, code)
	code, body = inst.do(t, http.MethodPatch, networkPath, admin, map[string]any{"enabled": true, "proxyUrl": proxyURL})
	require.Equal(t, http.StatusOK, code, "%s", body)
	require.JSONEq(t, `{"enabled":true,"hasProxyUrl":true,"source":"settings"}`, string(body))

	inst.shutdown(t.Context())
	opts.instanceURL = "https://fallback.example.test:9443/notes"
	inst = bootInstance(t.Context(), t, opts)
	admin = inst.signIn(t)
	code, body = inst.do(t, http.MethodGet, notificationPath, admin, nil)
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, string(body), `"callbackUrl":"`+publicCallback+`"`)
	require.Contains(t, string(body), `"wecomDefaultCallbackUrl":"`+defaultCallback+`"`)
	code, body = inst.do(t, http.MethodPatch, notificationPath, admin, map[string]any{"wecom": map[string]any{
		"enabled": false, "corpId": "wwfixture", "agentId": "1000001", "userId": "fixture", "callbackUrl": "",
	}})
	require.Equal(t, http.StatusOK, code, "%s", body)
	require.Contains(t, string(body), `"callbackUrl":""`)
	require.Contains(t, string(body), `"wecomDefaultCallbackUrl":"`+defaultCallback+`"`)
	require.Contains(t, string(body), `"hasCallbackToken":true`)
	code, _ = inst.do(t, http.MethodGet, callbackPath, "", nil)
	require.Equal(t, http.StatusNotFound, code, "a saved disable must override legacy environment settings on restart")
	code, body = inst.do(t, http.MethodGet, networkPath, admin, nil)
	require.Equal(t, http.StatusOK, code)
	require.JSONEq(t, `{"enabled":true,"hasProxyUrl":true,"source":"settings"}`, string(body))
	code, body = inst.do(t, http.MethodPatch, networkPath, admin, map[string]bool{"enabled": false})
	require.Equal(t, http.StatusOK, code)
	require.JSONEq(t, `{"enabled":false,"hasProxyUrl":true,"source":"settings"}`, string(body))
}
