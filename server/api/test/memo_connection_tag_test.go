package test

import (
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/proto/gen/api/apiconnect"
	"github.com/usememos/memos/store"
)

func TestBrowserMemoDoesNotAddConnectionTag(t *testing.T) {
	ctx := t.Context()
	ts := NewTestService(t)
	t.Cleanup(ts.Cleanup)
	ts.Profile.Personal = true
	server := httptest.NewServer(ts.Service.NewHandler())
	t.Cleanup(server.Close)
	client := server.Client()

	const password = "memo connection tag password"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	user, err := ts.Store.CreateUser(ctx, &store.User{
		Username: "memo-connection-owner", Role: store.RoleAdmin, PasswordHash: string(hash),
	})
	require.NoError(t, err)
	signedIn, err := apiconnect.NewAuthServiceClient(client, server.URL).SignIn(ctx, connect.NewRequest(&apipb.SignInRequest{
		Credentials: &apipb.SignInRequest_PasswordCredentials_{
			PasswordCredentials: &apipb.SignInRequest_PasswordCredentials{Username: user.Username, Password: password},
		},
	}))
	require.NoError(t, err)

	const content = "一条带 #项目 的笔记"
	memoClient := apiconnect.NewMemoServiceClient(client, server.URL)
	create := func() *apipb.Memo {
		t.Helper()
		request := connect.NewRequest(&apipb.CreateMemoRequest{
			MemoId: "browser-note", Idempotent: true, Memo: &apipb.Memo{Content: content},
		})
		request.Header().Set("Authorization", "Bearer "+signedIn.Msg.GetAccessToken())
		response, err := memoClient.CreateMemo(ctx, request)
		require.NoError(t, err)
		return response.Msg
	}

	created, replayed := create(), create()
	require.Equal(t, content, created.GetContent())
	require.Equal(t, []string{"项目"}, created.GetTags())
	require.Equal(t, created.GetName(), replayed.GetName())
	require.Equal(t, created.GetRevision(), replayed.GetRevision())
}
