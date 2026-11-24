package test

import (
	"context"
	"testing"

	"github.com/lithammer/shortuuid/v4"
	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestMemoRelationMutationRejectsCommentPersistence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()
	user, err := createTestingHostUser(ctx, ts)
	require.NoError(t, err)
	source, err := ts.CreateMemo(ctx, &store.Memo{UID: shortuuid.New(), CreatorID: user.ID, Content: "source", Visibility: store.Private})
	require.NoError(t, err)
	target, err := ts.CreateMemo(ctx, &store.Memo{UID: shortuuid.New(), CreatorID: user.ID, Content: "target", Visibility: store.Private})
	require.NoError(t, err)

	_, err = ts.UpsertMemoRelation(ctx, &store.MemoRelation{MemoID: source.ID, RelatedMemoID: target.ID, Type: store.MemoRelationComment})
	require.Error(t, err)
	err = ts.ApplyMemoMutation(ctx, &store.MemoMutation{
		MemoID:                    source.ID,
		MemoCreatorID:             user.ID,
		ExpectedMemoContent:       source.Content,
		ReplaceReferenceRelations: true,
		ReferenceRelations: []*store.MemoRelation{{
			MemoID: source.ID, RelatedMemoID: target.ID, Type: store.MemoRelationComment,
		}},
	})
	require.Error(t, err)

	relations, err := ts.ListMemoRelations(ctx, &store.FindMemoRelation{MemoID: &source.ID})
	require.NoError(t, err)
	require.Empty(t, relations)
}
