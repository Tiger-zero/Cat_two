package store_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestMemoPoliciesRequireOwnership(t *testing.T) {
	for _, visibility := range []store.Visibility{store.Private, store.Public, store.Protected} {
		t.Run(string(visibility), func(t *testing.T) {
			snapshot := &store.MemoRelationEndpointSnapshot{
				ActorUserID: 1, Actor: store.MemoActorState{Active: true, Admin: true}, EndpointID: 10,
				EndpointCreatorID: 1, EndpointRowStatus: store.Normal, EndpointVisibility: visibility,
			}
			require.NoError(t, store.ValidateMemoRelationEndpointRead(snapshot))
			snapshot.EndpointCreatorID = 2
			require.ErrorIs(t, store.ValidateMemoRelationEndpointRead(snapshot), store.ErrMemoPermissionDenied)

			policy := &store.MemoWritePolicy{ActorUserID: 1}
			write := &store.MemoWriteSnapshot{CreatorID: 1, RowStatus: store.Normal, Visibility: visibility, ActorIsAdmin: true}
			require.NoError(t, store.ValidateMemoWriteSnapshot(policy, nil, write))
			require.ErrorIs(t, store.ValidateMemoWriteSnapshot(policy, &store.UpdateMemo{Visibility: new(store.Public)}, write), store.ErrMemoPermissionDenied)
			write.CreatorID = 2
			require.ErrorIs(t, store.ValidateMemoWriteSnapshot(policy, nil, write), store.ErrMemoPermissionDenied)
		})
	}
}
