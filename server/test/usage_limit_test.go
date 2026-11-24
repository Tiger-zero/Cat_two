package test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	storepb "github.com/usememos/memos/proto/gen/store"
)

// Legacy usage limits remain readable for compatibility, but do not block
// memo creation over either transport.
func TestStoredUsageLimitsDoNotRestrictEitherTransport(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{personal: true, instanceURL: "http://127.0.0.1"})
	inst.createAdmin(t)
	token := inst.signIn(t)

	send := func(method, path string, body any, headers map[string]string) *http.Response {
		t.Helper()
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(ctx, method, inst.baseURL+path, bytes.NewReader(encoded))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		resp, err := inst.client.Do(req)
		require.NoError(t, err)
		return resp
	}

	_, err := inst.server.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_USAGE_LIMITS,
		Value: &storepb.InstanceSetting_UsageLimitsSetting{
			UsageLimitsSetting: &storepb.InstanceUsageLimitsSetting{
				Limits: []*storepb.InstanceUsageLimitsSetting_UsageLimit{{
					AllUsers:  true,
					MemoCount: new(int64(1)), MemoContentBytes: new(int64(8192)), UploadBytes: new(int64(33554432)),
				}},
			},
		},
	})
	require.NoError(t, err)

	status, body := inst.do(t, http.MethodPatch, "/api/instance/settings/USAGE_LIMITS", token, map[string]any{
		"usageLimitsSetting": map[string]any{
			"limits": []map[string]any{{"allUsers": true, "memoCount": "100"}},
		},
	})
	require.Equal(t, http.StatusNotImplemented, status, "%s", body)

	status, body = inst.do(t, http.MethodGet, "/api/instance/settings/USAGE_LIMITS", token, nil)
	require.Equal(t, http.StatusOK, status, "%s", body)
	var setting struct {
		Name               string `json:"name"`
		UsageLimitsSetting struct {
			Limits []struct {
				AllUsers         bool   `json:"allUsers"`
				MemoCount        string `json:"memoCount"`
				MemoContentBytes string `json:"memoContentBytes"`
				UploadBytes      string `json:"uploadBytes"`
			} `json:"limits"`
		} `json:"usageLimitsSetting"`
	}
	require.NoError(t, json.Unmarshal(body, &setting))
	require.Equal(t, "instance/settings/USAGE_LIMITS", setting.Name)
	require.Len(t, setting.UsageLimitsSetting.Limits, 1)
	require.True(t, setting.UsageLimitsSetting.Limits[0].AllUsers)
	require.Equal(t, "1", setting.UsageLimitsSetting.Limits[0].MemoCount)
	require.Equal(t, "8192", setting.UsageLimitsSetting.Limits[0].MemoContentBytes)
	require.Equal(t, "33554432", setting.UsageLimitsSetting.Limits[0].UploadBytes)

	inst.createMemo(t, token, "first", "the first memo")
	inst.createMemo(t, token, "second", "the second memo")
	memo := map[string]any{"content": "the third memo", "visibility": "PRIVATE"}
	connectResp := send(http.MethodPost, "/memos.api.MemoService/CreateMemo", map[string]any{"memo": memo}, map[string]string{"Connect-Protocol-Version": "1"})
	defer connectResp.Body.Close()
	require.Equal(t, http.StatusOK, connectResp.StatusCode)
	var created struct {
		Name    string `json:"name"`
		Content string `json:"content"`
	}
	require.NoError(t, json.NewDecoder(connectResp.Body).Decode(&created))
	require.NotEmpty(t, created.Name)
	require.Equal(t, "the third memo", created.Content)

	userName := "users/" + testAdminUsername
	status, body = inst.do(t, http.MethodGet, "/api/"+userName+"/usage", token, nil)
	require.Equal(t, http.StatusOK, status, "%s", body)
	var usage struct {
		Name       string                     `json:"name"`
		MemoCount  string                     `json:"memoCount"`
		Limits     map[string]json.RawMessage `json:"limits"`
		LimitIndex int                        `json:"limitIndex"`
	}
	require.NoError(t, json.Unmarshal(body, &usage))
	require.Equal(t, userName+"/usage", usage.Name)
	require.Equal(t, "3", usage.MemoCount)
	require.Empty(t, usage.Limits)
	require.Equal(t, -1, usage.LimitIndex)
}
