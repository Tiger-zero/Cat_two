package test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	storepb "github.com/usememos/memos/proto/gen/store"
)

func TestPersonalAdministratorSetupAndAccess(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{personal: true, instanceURL: "https://cat-two.example.test"})
	_, err := inst.server.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_ACCESS,
		Value: &storepb.InstanceSetting_AccessSetting{AccessSetting: &storepb.InstanceAccessSetting{
			AccessMode: storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PUBLIC,
		}},
	})
	require.NoError(t, err)
	_, err = inst.server.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_GENERAL,
		Value: &storepb.InstanceSetting_GeneralSetting{GeneralSetting: &storepb.InstanceGeneralSetting{
			DisallowPasswordAuth: true,
		}},
	})
	require.NoError(t, err)

	code, body := inst.do(t, http.MethodGet, "/api/instance/profile", "", nil)
	require.Equal(t, http.StatusOK, code, "%s", body)
	var instanceProfile struct {
		NeedsSetup bool   `json:"needsSetup"`
		AccessMode string `json:"accessMode"`
	}
	require.NoError(t, json.Unmarshal(body, &instanceProfile))
	require.True(t, instanceProfile.NeedsSetup)
	require.Equal(t, "INSTANCE_ACCESS_MODE_PRIVATE", instanceProfile.AccessMode)

	inst.createAdmin(t)
	session := inst.signIn(t)
	for _, bearer := range []string{"", session} {
		code, _ := inst.do(t, http.MethodPost, "/api/users", bearer, map[string]string{
			"username": "second-account", "password": testAdminPassword,
		})
		require.Equal(t, http.StatusForbidden, code)
	}
	code, _ = inst.do(t, http.MethodGet, "/api/users/"+testAdminUsername, "", nil)
	require.Equal(t, http.StatusUnauthorized, code)

	code, body = inst.do(t, http.MethodPost, "/api/users/"+testAdminUsername+"/personalAccessTokens", session, map[string]any{
		"description": "external client", "expiresInDays": 1,
	})
	require.Equal(t, http.StatusOK, code, "%s", body)
	var issued struct {
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(body, &issued))
	require.NotEmpty(t, issued.Token)
	code, _ = inst.do(t, http.MethodPatch, "/api/users/"+testAdminUsername+"?updateMask=password", issued.Token, map[string]string{
		"password": "different-password",
	})
	require.Equal(t, http.StatusForbidden, code)
	inst.signIn(t)
}
