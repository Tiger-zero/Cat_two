package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/server/auth"
)

// TestAuthorizerPrivateInstanceRegistration verifies that registration and other
// bootstrap methods stay reachable anonymously while unrelated public methods are
// gated. CreateUser enforces the instance registration settings in the service.
func TestAuthorizerPrivateInstanceRegistration(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	require.NoError(t, ts.SetInstanceAccessMode(ctx, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE))
	authorizer := api.NewAuthorizer(ts.Store, ts.Secret)

	const (
		createUser    = "/memos.api.UserService/CreateUser"
		signIn        = "/memos.api.AuthService/SignIn"
		listMemos     = "/memos.api.MemoService/ListMemos"
		listReactions = "/memos.api.MemoService/ListMemoReactions"
		listRelations = "/memos.api.MemoService/ListMemoRelations"
		getSharedMemo = "/memos.api.MemoService/GetSharedMemo"
	)

	// Anonymous request with no Authorization header resolves to no identity.
	require.Nil(t, authorizer.Authenticate(ctx, ""))

	// Registration and other bootstrap methods are allowed; browsing is still gated.
	require.NoError(t, authorizer.CheckAccess(ctx, createUser, nil))
	require.NoError(t, authorizer.CheckAccess(ctx, signIn, nil))
	require.NoError(t, authorizer.CheckAccess(ctx, getSharedMemo, nil))
	require.ErrorIs(t, authorizer.CheckAccess(ctx, listMemos, nil), api.ErrUnauthenticated)
	require.ErrorIs(t, authorizer.CheckAccess(ctx, listReactions, nil), api.ErrUnauthenticated)
	require.ErrorIs(t, authorizer.CheckAccess(ctx, listRelations, nil), api.ErrUnauthenticated)

	// Note endpoints require authentication regardless of instance access mode.
	require.NoError(t, ts.SetInstanceAccessMode(ctx, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PUBLIC))
	require.ErrorIs(t, authorizer.CheckAccess(ctx, listMemos, nil), api.ErrUnauthenticated)
	require.ErrorIs(t, authorizer.CheckAccess(ctx, listReactions, nil), api.ErrUnauthenticated)
	require.ErrorIs(t, authorizer.CheckAccess(ctx, listRelations, nil), api.ErrUnauthenticated)
	require.NoError(t, ts.SetInstanceAccessMode(ctx, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE))

	// Once a user exists, CreateUser remains reachable so UserService can enforce
	// disallow_user_registration and disallow_password_auth.
	_, err := ts.CreateHostUser(ctx, "host")
	require.NoError(t, err)
	require.NoError(t, authorizer.CheckAccess(ctx, createUser, nil))
	require.NoError(t, authorizer.CheckAccess(ctx, signIn, nil))
}

func TestAuthorizerAcceptsWebAccessTokenOnPrivateInstance(t *testing.T) {
	ctx := t.Context()
	ts := NewTestService(t)
	t.Cleanup(ts.Cleanup)
	user, err := ts.CreateRegularUser(ctx, "web-user")
	require.NoError(t, err)
	token, _, err := auth.GenerateAccessTokenV2(user.ID, user.Username, string(user.Role), string(user.RowStatus), user.PasswordHash, []byte(ts.Secret))
	require.NoError(t, err)

	require.NoError(t, ts.SetInstanceAccessMode(ctx, storepb.InstanceAccessMode_INSTANCE_ACCESS_MODE_PRIVATE))
	authorizer := api.NewAuthorizer(ts.Store, ts.Secret)
	result := authorizer.Authenticate(ctx, "Bearer "+token)
	require.NotNil(t, result)
	require.Equal(t, user.ID, result.Claims.UserID)
	require.NoError(t, authorizer.CheckAccess(ctx, "/memos.api.MemoService/ListMemos", result))
}
