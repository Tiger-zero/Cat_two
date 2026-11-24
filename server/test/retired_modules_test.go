package test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/proto/gen/api/apiconnect"
)

func TestPersonalServerRejectsRetiredModuleAccess(t *testing.T) {
	inst := bootInstance(t.Context(), t, instanceOptions{personal: true})
	inst.createAdmin(t)
	token := inst.signIn(t)
	parent := map[string]string{"parent": "users/" + testAdminUsername}

	for _, route := range []struct {
		procedure string
		body      any
	}{
		{"/memos.api.SpaceService/ListSpaces", nil},
		{"/memos.api.IdentityProviderService/ListIdentityProviders", nil},
		{"/memos.api.UserService/ListUserWebhooks", parent},
		{"/memos.api.UserService/GetUserWebhookSigningSecret", map[string]string{"name": "users/" + testAdminUsername + "/webhooks/removed"}},
		{"/memos.api.UserService/ListLinkedIdentities", parent},
	} {
		t.Run(route.procedure, func(t *testing.T) {
			for _, credential := range []string{"", token} {
				status, body := inst.connect(t, route.procedure, credential, route.body)
				require.Equal(t, http.StatusNotFound, status, "%s", body)
			}
		})
	}

	for _, route := range []struct {
		procedure string
		body      any
	}{
		{apiconnect.InstanceServiceGetConfigurationStatusProcedure, nil},
		{apiconnect.InstanceServiceGetInstanceSettingProcedure, map[string]string{"name": "instance/settings/NOTIFICATION"}},
		{apiconnect.UserServiceListUserNotificationsProcedure, parent},
		{apiconnect.InstanceServiceTestInstanceEmailSettingProcedure, nil},
	} {
		t.Run(route.procedure, func(t *testing.T) {
			status, body := inst.connect(t, route.procedure, token, route.body)
			require.Equal(t, http.StatusNotImplemented, status, "%s", body)
			require.Contains(t, string(body), `"code":"unimplemented"`)
		})
	}

	status, body := inst.connect(t, apiconnect.AuthServiceSignInProcedure, "", map[string]any{
		"ssoCredentials": map[string]string{"identityProvider": "identity-providers/previous", "code": "retired"},
	})
	require.Equal(t, http.StatusBadRequest, status, "%s", body)
	require.NotContains(t, string(body), "accessToken")

	status, body = inst.connect(t, apiconnect.UserServiceListUserSettingsProcedure, token, parent)
	require.Equal(t, http.StatusOK, status, "%s", body)
}
