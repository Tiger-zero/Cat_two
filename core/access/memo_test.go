package access

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestCheckMemoReadIsCreatorOnlyForAllStoredAudiences(t *testing.T) {
	owner := &store.User{ID: 1, RowStatus: store.Normal, Role: store.RoleUser}
	other := &store.User{ID: 2, RowStatus: store.Normal, Role: store.RoleUser}
	admin := &store.User{ID: 3, RowStatus: store.Normal, Role: store.RoleAdmin}
	archived := &store.User{ID: owner.ID, RowStatus: store.Archived}
	spaceID := int32(7)
	parentUID := "legacy-context"

	for _, visibility := range []store.Visibility{store.Public, store.Protected, store.Private, store.SpaceAudience} {
		t.Run(string(visibility), func(t *testing.T) {
			memo := &store.Memo{ID: 10, CreatorID: owner.ID, RowStatus: store.Normal, Visibility: visibility, SpaceID: &spaceID, ParentUID: &parentUID}
			read := MemoReadContext{Memo: memo, CreatorValid: true}
			require.True(t, CheckMemoReadContext(withViewer(read, owner)).Allowed())
			require.Equal(t, MemoReadDenialPermission, CheckMemoReadContext(withViewer(read, other)).Denial)
			require.Equal(t, MemoReadDenialPermission, CheckMemoReadContext(withViewer(read, admin)).Denial)
			require.Equal(t, MemoReadDenialUnauthenticated, CheckMemoReadContext(withViewer(read, archived)).Denial)
			require.Equal(t, MemoReadDenialUnauthenticated, CheckMemoReadContext(withViewer(read, nil)).Denial)

			memo.RowStatus = store.Archived
			require.True(t, CheckMemoReadContext(withViewer(read, owner)).Allowed())
			require.Equal(t, MemoReadDenialNotFound, CheckMemoReadContext(withViewer(read, admin)).Denial)
		})
	}
}

func TestMemoAndAttachmentManagementRequireOwnership(t *testing.T) {
	owner := &store.User{ID: 1, RowStatus: store.Normal, Role: store.RoleUser}
	admin := &store.User{ID: 2, RowStatus: store.Normal, Role: store.RoleAdmin}
	archivedOwner := &store.User{ID: owner.ID, RowStatus: store.Archived}
	memo := &store.Memo{CreatorID: owner.ID}
	attachment := &store.Attachment{CreatorID: owner.ID}
	require.True(t, CanManageMemo(owner, memo))
	require.True(t, CanManageAttachment(owner, attachment))
	for _, viewer := range []*store.User{admin, archivedOwner, nil} {
		require.False(t, CanManageMemo(viewer, memo))
		require.False(t, CanManageAttachment(viewer, attachment))
	}
}

func TestCheckMemoReadInvalidStateFailsClosed(t *testing.T) {
	owner := &store.User{ID: 1, RowStatus: store.Normal}
	memo := &store.Memo{ID: 10, CreatorID: owner.ID, RowStatus: store.Normal, Visibility: store.Public}
	read := MemoReadContext{Memo: memo, CreatorValid: true}
	require.True(t, CheckMemoReadContext(withViewer(read, owner)).Allowed(), "legacy public note remains visible to its owner")

	memo.Visibility = store.SpaceAudience
	require.True(t, CheckMemoReadContext(withViewer(read, owner)).Allowed(), "a historical placement does not affect the author")
	memo.Visibility = store.Visibility("FUTURE_AUDIENCE")
	require.Equal(t, MemoReadDenialNotFound, CheckMemoReadContext(withViewer(read, owner)).Denial)
	memo.Visibility = store.Public
	memo.RowStatus = store.RowStatus("UNKNOWN")
	require.Equal(t, MemoReadDenialNotFound, CheckMemoReadContext(withViewer(read, owner)).Denial)
	memo.RowStatus = store.Normal
	read.CreatorValid = false
	require.Equal(t, MemoReadDenialNotFound, CheckMemoReadContext(withViewer(read, owner)).Denial)
}

func withViewer(read MemoReadContext, viewer *store.User) MemoReadContext {
	read.Viewer = viewer
	return read
}
