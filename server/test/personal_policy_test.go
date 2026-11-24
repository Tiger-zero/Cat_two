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

	code, body := inst.connect(t, apiconnect.InstanceServiceGetInstanceProfileProcedure, "", nil)
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
	code, body = inst.connect(t, apiconnect.InstanceServiceUpdateInstanceSettingProcedure, session, map[string]any{
		"instanceSetting": map[string]any{
			"name": "instance/settings/GENERAL", "generalSetting": map[string]bool{"disallowPasswordAuth": true},
		},
	})
	require.Equal(t, http.StatusBadRequest, code, "%s", body)
	for _, bearer := range []string{"", session} {
		code, body := inst.connect(t, apiconnect.UserServiceCreateUserProcedure, bearer, map[string]any{
			"user": map[string]string{"username": "second-account", "password": testAdminPassword},
		})
		require.Equal(t, http.StatusForbidden, code, "%s", body)
	}
	userName := "users/" + testAdminUsername
	code, body = inst.connect(t, apiconnect.UserServiceGetUserProcedure, "", map[string]string{"name": userName})
	require.Equal(t, http.StatusUnauthorized, code, "%s", body)

	_, mcpToken := createMCPConnection(t, inst, session, "Browser authorization fixture", "")
	code, body = inst.connect(t, apiconnect.UserServiceUpdateUserProcedure, mcpToken, map[string]any{
		"user": map[string]string{"name": userName, "password": "different-password"}, "updateMask": "password",
	})
	require.Equal(t, http.StatusUnauthorized, code, "%s", body)
	inst.signIn(t)
}
