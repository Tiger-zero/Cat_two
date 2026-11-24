package test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestReactionWritePolicyRejectsInactiveActorAndMemo(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "reaction-state-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	actor, err := ts.CreateUser(ctx, &store.User{Username: "reaction-state-actor", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	memo, err := ts.CreateMemo(ctx, &store.Memo{UID: "reaction-state-memo", CreatorID: owner.ID, Content: "memo", Visibility: store.Public})
	require.NoError(t, err)

	archived := store.Archived
	_, err = ts.UpdateUser(ctx, &store.UpdateUser{ID: actor.ID, RowStatus: &archived})
	require.NoError(t, err)
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: actor.ID, MemoID: memo.ID, ReactionType: "inactive-actor", Policy: reactionWritePolicy(actor.ID),
	})
	require.ErrorIs(t, err, store.ErrReactionPermissionDenied)

	require.NoError(t, ts.UpdateMemo(ctx, &store.UpdateMemo{ID: memo.ID, RowStatus: &archived}))
	_, err = ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: owner.ID, MemoID: memo.ID, ReactionType: "inactive-memo", Policy: reactionWritePolicy(owner.ID),
	})
	require.ErrorIs(t, err, store.ErrMemoContextUnavailable)
}

func TestDeleteReactionAtomicallyEnforcesCreatorWithoutParticipation(t *testing.T) {
	ctx := context.Background()
	ts := NewTestingStore(ctx, t)
	defer ts.Close()

	owner, err := ts.CreateUser(ctx, &store.User{Username: "reaction-delete-owner", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	member, err := ts.CreateUser(ctx, &store.User{Username: "reaction-delete-member", Role: store.RoleUser, PasswordHash: "hash"})
	require.NoError(t, err)
	memo, err := ts.CreateMemo(ctx, &store.Memo{
		UID: "reaction-delete-memo", CreatorID: owner.ID, Content: "memo", Visibility: store.Public,
	})
	require.NoError(t, err)
	reaction, err := ts.UpsertReaction(ctx, &store.Reaction{
		CreatorID: member.ID, MemoID: memo.ID, ReactionType: "delete",
	})
	require.NoError(t, err)

	err = ts.DeleteReaction(ctx, &store.DeleteReaction{
		ID: &reaction.ID, MemoID: &memo.ID, ActorUserID: &owner.ID, Policy: reactionWritePolicy(owner.ID),
	})
	require.ErrorIs(t, err, store.ErrReactionPermissionDenied)
	requireReactionPresent(ctx, t, ts, reaction.ID)

	err = ts.DeleteReaction(ctx, &store.DeleteReaction{
		ID: &reaction.ID, MemoID: &memo.ID, ActorUserID: &member.ID, Policy: reactionWritePolicy(member.ID),
	})
	require.NoError(t, err, "creators can withdraw reactions on legacy notes they did not author")

	requireReactionMissing(ctx, t, ts, reaction.ID)
}

func reactionWritePolicy(actorUserID int32) *store.ReactionWritePolicy {
	return &store.ReactionWritePolicy{ActorUserID: actorUserID}
}

func requireReactionPresent(ctx context.Context, t *testing.T, ts *store.Store, reactionID int32) {
	t.Helper()
	reaction, err := ts.GetReaction(ctx, &store.FindReaction{ID: &reactionID})
	require.NoError(t, err)
	require.NotNil(t, reaction)
}

func requireReactionMissing(ctx context.Context, t *testing.T, ts *store.Store, reactionID int32) {
	t.Helper()
	reaction, err := ts.GetReaction(ctx, &store.FindReaction{ID: &reactionID})
	require.NoError(t, err)
	require.Nil(t, reaction)
}
