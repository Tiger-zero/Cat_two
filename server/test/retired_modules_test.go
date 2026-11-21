package test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

func TestPersonalServerRejectsRetiredModuleAccess(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	token := inst.signIn(t)
	username := testAdminUsername
	user, err := inst.server.Store.GetUser(ctx, &store.FindUser{Username: &username})
	require.NoError(t, err)
	require.NoError(t, inst.server.Store.AddUserWebhook(ctx, user.ID, &storepb.WebhooksUserSetting_Webhook{
		Id: "legacy-hook", Url: "https://example.test/saved-webhook", SigningSecret: "legacy-signing-secret",
	}))

	for _, tc := range []struct {
		path string
		rpc  string
		body map[string]string
	}{
		{path: "/api/spaces", rpc: "SpaceService/ListSpaces"},
		{path: "/api/identity-providers", rpc: "IdentityProviderService/ListIdentityProviders"},
		{path: "/api/instance/configuration", rpc: "InstanceService/GetConfigurationStatus"},
		{path: "/api/instance/settings/NOTIFICATION", rpc: "InstanceService/GetInstanceSetting", body: map[string]string{"name": "instance/settings/NOTIFICATION"}},
		{path: "/api/users/" + username + "/notifications", rpc: "UserService/ListUserNotifications", body: map[string]string{"parent": "users/" + username}},
		{path: "/api/users/" + username + "/webhooks", rpc: "UserService/ListUserWebhooks", body: map[string]string{"parent": "users/" + username}},
		{path: "/api/users/" + username + "/webhooks/legacy-hook:getSigningSecret", rpc: "UserService/GetUserWebhookSigningSecret", body: map[string]string{"name": "users/" + username + "/webhooks/legacy-hook"}},
		{path: "/api/users/" + username + "/linkedIdentities", rpc: "UserService/ListLinkedIdentities", body: map[string]string{"parent": "users/" + username}},
	} {
		t.Run(tc.rpc, func(t *testing.T) {
			status, body := inst.do(t, http.MethodGet, tc.path, token, nil)
			require.Equal(t, http.StatusNotImplemented, status, "%s", body)
			request := tc.body
			if request == nil {
				request = map[string]string{}
			}
			status, body = inst.do(t, http.MethodPost, "/memos.api."+tc.rpc, token, request)
			require.Equal(t, http.StatusNotImplemented, status, "%s", body)
		})
	}

	status, body := inst.do(t, http.MethodPost, "/api/auth/signin", "", map[string]any{
		"ssoCredentials": map[string]string{"identityProvider": "identity-providers/legacy", "code": "legacy"},
	})
	require.Equal(t, http.StatusNotImplemented, status, "%s", body)

	status, body = inst.do(t, http.MethodGet, "/api/users/"+username+"/settings", token, nil)
	require.Equal(t, http.StatusOK, status, "%s", body)
	require.NotContains(t, string(body), "saved-webhook")
	hooks, err := inst.server.Store.GetUserWebhooks(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, hooks, 1, "legacy data stays in the database")
}

func TestPersonalServerRetainsExistingSpaceNotesWithoutMembership(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	adminName := testAdminUsername
	admin, err := inst.server.Store.GetUser(ctx, &store.FindUser{Username: &adminName})
	require.NoError(t, err)
	space, err := inst.server.Store.CreateSpace(ctx, &store.Space{UID: "held-space", Title: "Historical space"}, admin.ID)
	require.NoError(t, err)
	const username = "legacy-author"
	const password = "legacy-password"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	_, err = inst.server.Store.CreateUser(ctx, &store.User{Username: username, Role: store.RoleUser, PasswordHash: string(hash)})
	require.NoError(t, err)
	token := inst.signInUser(t, username, password)
	inst.createMemo(t, token, "old-space-note", "memo from before retirement")
	// Reproduce a regular author's historical placement after membership removal.
	_, err = inst.server.Store.GetDriver().GetDB().ExecContext(ctx, "UPDATE memo SET space_id = ? WHERE uid = ?", space.ID, "old-space-note")
	require.NoError(t, err)
	inst.requireMemo(t, token, "old-space-note", "memo from before retirement")

	const memo = "/api/memos/old-space-note"
	status, body := inst.do(t, http.MethodPatch, memo+"?updateMask=content,space", token, map[string]string{
		"content": "still editable", "space": "spaces/held-space",
	})
	require.Equal(t, http.StatusOK, status, "%s", body)
	inst.requireMemo(t, token, "old-space-note", "still editable")

	status, body = inst.do(t, http.MethodPost, memo+"/reactions", token, map[string]any{
		"reaction": map[string]string{"reactionType": "👍"},
	})
	require.Equal(t, http.StatusOK, status, "%s", body)
	status, body = inst.do(t, http.MethodPost, memo+"/comments", token, map[string]string{"content": "still commentable"})
	require.Equal(t, http.StatusOK, status, "%s", body)

	status, body = inst.do(t, http.MethodPatch, memo+"?updateMask=space", token, map[string]string{"space": ""})
	require.Equal(t, http.StatusNotImplemented, status, "%s", body)
	status, body = inst.do(t, http.MethodPost, "/api/memos", token, map[string]string{"content": "new note", "space": "spaces/held-space"})
	require.Equal(t, http.StatusNotImplemented, status, "%s", body)
}
