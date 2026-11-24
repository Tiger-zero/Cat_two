package test

import (
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/internal/testutil"
	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/proto/gen/api/apiconnect"
	"github.com/usememos/memos/server/auth"
)

func TestLegacyPersonalAccessTokenCannotAuthenticate(t *testing.T) {
	ctx := t.Context()
	ts := NewTestService(t)
	t.Cleanup(ts.Cleanup)
	user, err := ts.CreateHostUser(ctx, "retired-token-owner")
	require.NoError(t, err)
	const token = "memos_pat_test-retired-credential"
	historical := testutil.LegacyPATValue(token)
	_, err = ts.Store.GetDriver().GetDB().ExecContext(ctx,
		"INSERT INTO user_setting (user_id, key, value) VALUES (?, 'PERSONAL_ACCESS_TOKENS', ?)", user.ID, historical)
	require.NoError(t, err)

	authenticator := auth.NewAuthenticator(ts.Store, ts.Secret)
	require.Nil(t, authenticator.Authenticate(ctx, "Bearer "+token))
	require.False(t, authenticator.VerifyBearer(ctx, "Bearer "+token))
	fileUser, err := authenticator.AuthenticateToUser(ctx, "Bearer "+token, "")
	require.NoError(t, err)
	require.Nil(t, fileUser)

	server := httptest.NewServer(ts.Service.NewHandler())
	t.Cleanup(server.Close)
	client := apiconnect.NewMemoServiceClient(server.Client(), server.URL)
	req := connect.NewRequest(&apipb.ListMemosRequest{})
	req.Header().Set("Authorization", "Bearer "+token)
	_, err = client.ListMemos(ctx, req)
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))

	var stored string
	err = ts.Store.GetDriver().GetDB().QueryRowContext(ctx,
		"SELECT value FROM user_setting WHERE user_id = ? AND key = 'PERSONAL_ACCESS_TOKENS'", user.ID).Scan(&stored)
	require.NoError(t, err)
	require.Equal(t, historical, stored)
}
