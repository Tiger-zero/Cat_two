package test

import (
	"net/http"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/proto/gen/api/apiconnect"
)

func TestMemoReplayAndRevisionAcrossConnectEncodings(t *testing.T) {
	ctx := t.Context()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	token := inst.signIn(t)
	const memoID, content = "transport-retry", "First write over Connect JSON"
	code, body := inst.connect(t, apiconnect.MemoServiceCreateMemoProcedure, token, map[string]any{
		"memoId": memoID, "idempotent": true, "memo": map[string]string{"content": content},
	})
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

	code, body = inst.connect(t, apiconnect.MemoServiceUpdateMemoProcedure, token, map[string]any{
		"memo":       map[string]string{"name": memo.Name, "content": "Stale JSON write"},
		"updateMask": "content", "expectedRevision": memo.Revision,
	})
	require.Equal(t, http.StatusConflict, code, "%s", body)
	code, body = inst.connect(t, apiconnect.MemoServiceGetMemoProcedure, token, map[string]string{"name": memo.Name})
	require.Equal(t, http.StatusOK, code, "%s", body)
	var current apipb.Memo
	require.NoError(t, protojson.Unmarshal(body, &current))
	require.Equal(t, changed, current.Content)
	require.Equal(t, updated.Msg.Revision, current.Revision)
}
