package test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/proto/gen/api/apiconnect"
	storepb "github.com/usememos/memos/proto/gen/store"
)

func TestPersonalGeneralSettingsRejectAndHideCustomJavaScript(t *testing.T) {
	ctx := t.Context()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	browserToken := inst.signIn(t)
	code, body := inst.do(t, http.MethodPost, "/api/users/"+testAdminUsername+"/personalAccessTokens", browserToken, map[string]string{
		"description": "legacy settings client",
	})
	require.Equal(t, http.StatusOK, code, "%s", body)
	var pat struct{ Token string }
	require.NoError(t, json.Unmarshal(body, &pat))
	require.NotEmpty(t, pat.Token)

	const legacyScript = "legacy script stored in an older release"
	const brand = "Personal workspace"
	_, err := inst.server.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_GENERAL,
		Value: &storepb.InstanceSetting_GeneralSetting{GeneralSetting: &storepb.InstanceGeneralSetting{
			AdditionalScript: legacyScript,
			AdditionalStyle:  "body { color: black; }",
			CustomProfile:    &storepb.InstanceCustomProfile{Title: brand},
		}},
	})
	require.NoError(t, err)

	const name = "instance/settings/GENERAL"
	const path = "/api/" + name
	for _, request := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, path, nil},
		{http.MethodPost, "/api/instance/settings:batchGet", map[string]any{"names": []string{name}}},
		{http.MethodPost, apiconnect.InstanceServiceGetInstanceSettingProcedure, map[string]any{"name": name}},
	} {
		code, body := inst.do(t, request.method, request.path, "", request.body)
		require.Equal(t, http.StatusOK, code, "%s", body)
		require.Contains(t, string(body), brand)
		require.Contains(t, string(body), "body { color: black; }")
		require.NotContains(t, string(body), legacyScript)
	}

	const attemptedScript = "new custom script value"
	setting := map[string]any{"name": name, "generalSetting": map[string]string{"additionalScript": attemptedScript}}
	for _, credential := range []string{browserToken, pat.Token} {
		for _, request := range []struct {
			method, path string
			body         any
		}{
			{http.MethodPatch, path, setting},
			{http.MethodPost, apiconnect.InstanceServiceUpdateInstanceSettingProcedure, map[string]any{"instanceSetting": setting}},
		} {
			code, body := inst.do(t, request.method, request.path, credential, request.body)
			require.Equal(t, http.StatusBadRequest, code, "%s", body)
			require.Contains(t, string(body), "custom JavaScript is unavailable in Cat_two")
			require.NotContains(t, string(body), attemptedScript)
		}
	}

	code, body = inst.do(t, http.MethodPatch, path, pat.Token, map[string]any{
		"generalSetting": map[string]string{"additionalStyle": "body { color: blue; }"},
	})
	require.Equal(t, http.StatusOK, code, "%s", body)
	updated, err := inst.server.Store.GetInstanceGeneralSetting(ctx)
	require.NoError(t, err)
	require.Empty(t, updated.GetAdditionalScript())
	require.Equal(t, "body { color: blue; }", updated.GetAdditionalStyle())
}
