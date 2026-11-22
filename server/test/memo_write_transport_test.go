package test

import (
	"net/http"
	"net/url"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/proto/gen/api/apiconnect"
)

func TestMemoReplayAndRevisionAcrossRESTAndConnect(t *testing.T) {
	ctx := t.Context()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	token := inst.signIn(t)
	const memoID, content = "transport-retry", "First write over REST"
	code, body := inst.do(t, http.MethodPost, "/api/memos?memoId="+memoID+"&idempotent=true", token, map[string]string{"content": content})
	require.Equal(t, http.StatusOK, code, "%s", body)
	var memo apipb.Memo
	require.NoError(t, protojson.Unmarshal(body, &memo))
	require.NotEmpty(t, memo.Revision)

	client := apiconnect.NewMemoServiceClient(inst.client, inst.baseURL)
	create := connect.NewRequest(&apipb.CreateMemoRequest{MemoId: memoID, Idempotent: true, Memo: &apipb.Memo{Content: content}})
	create.Header().Set("Authorization", "Bearer "+token)
	replay, err := client.CreateMemo(ctx, create)
	require.NoError(t, err)
	require.Equal(t, memo.Name, replay.Msg.Name)
	require.Equal(t, memo.Revision, replay.Msg.Revision)

	const changed = "Conditional write over Connect"
	update := connect.NewRequest(&apipb.UpdateMemoRequest{
		Memo:             &apipb.Memo{Name: memo.Name, Content: changed},
		UpdateMask:       &fieldmaskpb.FieldMask{Paths: []string{"content"}},
		ExpectedRevision: replay.Msg.Revision,
	})
	update.Header().Set("Authorization", "Bearer "+token)
	updated, err := client.UpdateMemo(ctx, update)
	require.NoError(t, err)
	require.NotEqual(t, memo.Revision, updated.Msg.Revision)

	path := "/api/" + memo.Name
	code, body = inst.do(t, http.MethodPatch, path+"?updateMask=content&expectedRevision="+url.QueryEscape(memo.Revision), token, map[string]string{"content": "Stale REST write"})
	require.Equal(t, http.StatusConflict, code, "%s", body)
	code, body = inst.do(t, http.MethodGet, path, token, nil)
	require.Equal(t, http.StatusOK, code, "%s", body)
	var current apipb.Memo
	require.NoError(t, protojson.Unmarshal(body, &current))
	require.Equal(t, changed, current.Content)
	require.Equal(t, updated.Msg.Revision, current.Revision)
}
