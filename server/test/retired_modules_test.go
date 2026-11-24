package test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/usememos/memos/internal/testutil/legacyspace"
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
	db := inst.server.Store.GetDriver().GetDB()
	_, err = db.ExecContext(ctx, "INSERT INTO idp (uid, name, type) VALUES ('legacy', 'Legacy provider', 'OAUTH2')")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "INSERT INTO user_identity (user_id, provider, extern_uid) VALUES (?, 'legacy', 'legacy-subject')", user.ID)
	require.NoError(t, err)
	require.NoError(t, inst.server.Store.AddUserWebhook(ctx, user.ID, &storepb.WebhooksUserSetting_Webhook{
		Id: "legacy-hook", Url: "https://example.test/saved-webhook", SigningSecret: "legacy-signing-secret",
	}))

	for _, tc := range []struct {
		path, rpc string
		body      map[string]string
		anonymous bool
	}{
		{path: "/api/spaces", rpc: "SpaceService/ListSpaces", anonymous: true},
		{path: "/api/identity-providers", rpc: "IdentityProviderService/ListIdentityProviders", anonymous: true},
		{path: "/api/instance/configuration", rpc: "InstanceService/GetConfigurationStatus", anonymous: true},
		{path: "/api/instance/settings/NOTIFICATION", rpc: "InstanceService/GetInstanceSetting", body: map[string]string{"name": "instance/settings/NOTIFICATION"}},
		{path: "/api/users/" + username + "/notifications", rpc: "UserService/ListUserNotifications", body: map[string]string{"parent": "users/" + username}, anonymous: true},
		{path: "/api/users/" + username + "/webhooks", rpc: "UserService/ListUserWebhooks", body: map[string]string{"parent": "users/" + username}, anonymous: true},
		{path: "/api/users/" + username + "/webhooks/legacy-hook:getSigningSecret", rpc: "UserService/GetUserWebhookSigningSecret", body: map[string]string{"name": "users/" + username + "/webhooks/legacy-hook"}, anonymous: true},
		{path: "/api/users/" + username + "/linkedIdentities", rpc: "UserService/ListLinkedIdentities", body: map[string]string{"parent": "users/" + username}, anonymous: true},
	} {
		t.Run(tc.rpc, func(t *testing.T) {
			request := tc.body
			if request == nil {
				request = map[string]string{}
			}
			tokens := []string{token}
			if tc.anonymous {
				tokens = append(tokens, "")
			}
			for _, caller := range tokens {
				status, body := inst.do(t, http.MethodGet, tc.path, caller, nil)
				require.Equal(t, http.StatusNotImplemented, status, "%s", body)
				require.Contains(t, string(body), "this feature is unavailable in Cat_two")
				status, body = inst.do(t, http.MethodPost, "/memos.api."+tc.rpc, caller, request)
				require.Equal(t, http.StatusNotImplemented, status, "%s", body)
				require.Contains(t, string(body), `"code":"unimplemented"`)
			}
		})
	}

	status, body := inst.do(t, http.MethodPost, "/api/auth/signin", "", map[string]any{
		"ssoCredentials": map[string]string{"identityProvider": "identity-providers/legacy", "code": "legacy"},
	})
	require.Equal(t, http.StatusNotImplemented, status, "%s", body)
	status, body = inst.do(t, http.MethodPost, "/api/instance/settings/notification:testEmail", token, map[string]string{})
	require.Equal(t, http.StatusNotImplemented, status, "%s", body)
	status, body = inst.do(t, http.MethodPatch, "/api/instance/settings/USAGE_LIMITS", token, map[string]any{
		"usageLimitsSetting": map[string]any{"limits": []map[string]any{{"allUsers": true, "memoCount": "1"}}},
	})
	require.Equal(t, http.StatusNotImplemented, status, "%s", body)

	status, body = inst.do(t, http.MethodGet, "/api/users/"+username+"/settings", token, nil)
	require.Equal(t, http.StatusOK, status, "%s", body)
	require.NotContains(t, string(body), "saved-webhook")
	hooks, err := inst.server.Store.GetUserWebhooks(ctx, user.ID)
	require.NoError(t, err)
	require.Len(t, hooks, 1, "legacy data stays in the database")
	require.Equal(t, "legacy-signing-secret", hooks[0].GetSigningSecret())
	var subject string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT extern_uid FROM user_identity WHERE user_id = ? AND provider = 'legacy'", user.ID).Scan(&subject))
	require.Equal(t, "legacy-subject", subject)
}

func TestPersonalServerRetainsExistingSpaceNotesWithoutMembership(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	adminName := testAdminUsername
	admin, err := inst.server.Store.GetUser(ctx, &store.FindUser{Username: &adminName})
	require.NoError(t, err)
	db := inst.server.Store.GetDriver().GetDB()
	spaceID, err := legacyspace.Seed(ctx, db, "held-space", "Historical space", admin.ID)
	require.NoError(t, err)
	const username = "legacy-author"
	const password = "legacy-password"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	_, err = inst.server.Store.CreateUser(ctx, &store.User{Username: username, Role: store.RoleUser, PasswordHash: string(hash)})
	require.NoError(t, err)
	token := inst.signInUser(t, username, password)
	inst.createMemo(t, token, "old-space-note", "memo from before retirement")
	// Reproduce a regular author's historical placement in another user's Space.
	_, err = db.ExecContext(ctx, "UPDATE memo SET space_id = ?, visibility = 'SPACE' WHERE uid = ?", spaceID, "old-space-note")
	require.NoError(t, err)
	inst.requireMemo(t, token, "old-space-note", "memo from before retirement")

	const memo = "/api/memos/old-space-note"
	status, body := inst.do(t, http.MethodGet, memo, inst.signIn(t), nil)
	require.Equal(t, http.StatusForbidden, status, "%s", body)
	status, body = inst.do(t, http.MethodPatch, memo+"?updateMask=content,space", token, map[string]string{
		"content": "rejected update", "space": "spaces/held-space",
	})
	require.Equal(t, http.StatusNotImplemented, status, "%s", body)
	inst.requireMemo(t, token, "old-space-note", "memo from before retirement")
	status, body = inst.do(t, http.MethodPatch, memo+"?updateMask=content,space", token, map[string]string{
		"content": "still editable", "space": "",
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
	require.Equal(t, http.StatusOK, status, "%s", body)
	var storedSpaceID int32
	require.NoError(t, db.QueryRowContext(ctx, "SELECT space_id FROM memo WHERE uid = ?", "old-space-note").Scan(&storedSpaceID))
	require.Equal(t, spaceID, storedSpaceID)
	status, body = inst.do(t, http.MethodPost, "/api/memos", token, map[string]string{"content": "new note", "space": "spaces/held-space"})
	require.Equal(t, http.StatusNotImplemented, status, "%s", body)
	status, body = inst.do(t, http.MethodGet, "/api/memos?filter=space%20%3D%3D%20null", token, nil)
	require.Equal(t, http.StatusBadRequest, status, "%s", body)

	// A dangling placement used to prevent the author from reading a SPACE note.
	_, err = db.ExecContext(ctx, "UPDATE memo SET space_id = NULL WHERE uid = ?", "old-space-note")
	require.NoError(t, err)
	inst.requireMemo(t, token, "old-space-note", "still editable")
	status, body = inst.do(t, http.MethodGet, "/api/memos", token, nil)
	require.Equal(t, http.StatusOK, status, "%s", body)
	require.Contains(t, string(body), "still editable")
}
