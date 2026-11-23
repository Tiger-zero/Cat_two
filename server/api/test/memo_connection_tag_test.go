package test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/encoding/protojson"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/proto/gen/api/apiconnect"
	"github.com/usememos/memos/store"
)

func TestMemoConnectionTagThroughAPI(t *testing.T) {
	ctx := t.Context()
	ts := NewTestService(t)
	t.Cleanup(ts.Cleanup)
	ts.Profile.Personal = true
	handler, err := ts.Service.NewHandler(ctx)
	require.NoError(t, err)
	server := httptest.NewTestServer(t, handler)
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
	sessionToken := signedIn.Msg.GetAccessToken()
	require.NotEmpty(t, sessionToken)
	pat, err := ts.Service.CreatePersonalAccessToken(ts.CreateUserContext(ctx, user.ID), &apipb.CreatePersonalAccessTokenRequest{
		Parent: "users/" + user.Username, Description: "memo connection tag integration",
	})
	require.NoError(t, err)
	require.NotEmpty(t, pat.GetToken())

	memoClient := apiconnect.NewMemoServiceClient(client, server.URL)
	transports := []struct {
		name   string
		create func(t *testing.T, token, id, content string) *apipb.Memo
	}{
		{
			name: "rest",
			create: func(t *testing.T, token, id, content string) *apipb.Memo {
				t.Helper()
				body, err := protojson.Marshal(&apipb.Memo{Content: content})
				require.NoError(t, err)
				request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/memos?memoId="+id+"&idempotent=true", bytes.NewReader(body))
				require.NoError(t, err)
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Authorization", "Bearer "+token)
				response, err := client.Do(request)
				require.NoError(t, err)
				defer response.Body.Close()
				result, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, response.StatusCode, "%s", result)
				memo := new(apipb.Memo)
				require.NoError(t, protojson.Unmarshal(result, memo))
				return memo
			},
		},
		{
			name: "connect",
			create: func(t *testing.T, token, id, content string) *apipb.Memo {
				t.Helper()
				request := connect.NewRequest(&apipb.CreateMemoRequest{
					MemoId: id, Idempotent: true, Memo: &apipb.Memo{Content: content},
				})
				request.Header().Set("Authorization", "Bearer "+token)
				response, err := memoClient.CreateMemo(t.Context(), request)
				require.NoError(t, err)
				return response.Msg
			},
		},
	}
	for _, transport := range transports {
		t.Run(transport.name, func(t *testing.T) {
			const body = "一条带 #项目 的笔记"
			id := transport.name + "-pat"
			created := transport.create(t, pat.GetToken(), id, body)
			require.Equal(t, "memos/"+id, created.GetName())
			require.Equal(t, body+"\n\n#MCP连接", created.GetContent())
			require.ElementsMatch(t, []string{"项目", "MCP连接"}, created.GetTags())

			replayed := transport.create(t, pat.GetToken(), id, body)
			require.Equal(t, created.GetName(), replayed.GetName())
			require.Equal(t, created.GetRevision(), replayed.GetRevision())
			require.Equal(t, created.GetContent(), replayed.GetContent())
			require.ElementsMatch(t, created.GetTags(), replayed.GetTags())

			const alreadyTagged = "#MCP连接 开头的**原文**"
			existing := transport.create(t, pat.GetToken(), transport.name+"-existing-tag", alreadyTagged)
			require.Equal(t, alreadyTagged, existing.GetContent())
			require.Equal(t, []string{"MCP连接"}, existing.GetTags())

			fromSession := transport.create(t, sessionToken, transport.name+"-session", body)
			require.Equal(t, body, fromSession.GetContent())
			require.Equal(t, []string{"项目"}, fromSession.GetTags())
		})
	}
}
