package test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/proto/gen/api/apiconnect"
	storepb "github.com/usememos/memos/proto/gen/store"
)

// Legacy usage limits remain readable for compatibility, but do not block
// memo creation from the web client.
func TestStoredUsageLimitsDoNotRestrictWebMemoCreation(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{personal: true, instanceURL: "http://127.0.0.1"})
	inst.createAdmin(t)
	token := inst.signIn(t)

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

	const name = "instance/settings/USAGE_LIMITS"
	status, body := inst.connect(t, apiconnect.InstanceServiceUpdateInstanceSettingProcedure, token, map[string]any{
		"instanceSetting": map[string]any{
			"name": name,
			"usageLimitsSetting": map[string]any{
				"limits": []map[string]any{{"allUsers": true, "memoCount": "100"}},
			},
		},
	})
	require.Equal(t, http.StatusNotImplemented, status, "%s", body)

	status, body = inst.connect(t, apiconnect.InstanceServiceGetInstanceSettingProcedure, token, map[string]string{"name": name})
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
	inst.createMemo(t, token, "third", "the third memo")

	userName := "users/" + testAdminUsername
	status, body = inst.connect(t, apiconnect.UserServiceGetUserUsageProcedure, token, map[string]string{"name": userName + "/usage"})
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
