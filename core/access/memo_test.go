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
	parentUID := "legacy-context"

	for _, visibility := range []store.Visibility{store.Public, store.Protected, store.Private} {
		t.Run(string(visibility), func(t *testing.T) {
			memo := &store.Memo{ID: 10, CreatorID: owner.ID, Visibility: visibility, ParentUID: &parentUID}
			read := MemoReadContext{Memo: memo, CreatorValid: true}
			require.True(t, CheckMemoReadContext(withViewer(read, owner)).Allowed())
			require.Equal(t, MemoReadDenialPermission, CheckMemoReadContext(withViewer(read, other)).Denial)
			require.Equal(t, MemoReadDenialPermission, CheckMemoReadContext(withViewer(read, admin)).Denial)
			require.Equal(t, MemoReadDenialUnauthenticated, CheckMemoReadContext(withViewer(read, archived)).Denial)
			require.Equal(t, MemoReadDenialUnauthenticated, CheckMemoReadContext(withViewer(read, nil)).Denial)
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
	memo := &store.Memo{ID: 10, CreatorID: owner.ID, Visibility: store.Public}
	read := MemoReadContext{Memo: memo, CreatorValid: true}
	require.True(t, CheckMemoReadContext(withViewer(read, owner)).Allowed(), "legacy public note remains visible to its owner")

	memo.Visibility = store.Visibility("FUTURE_AUDIENCE")
	require.Equal(t, MemoReadDenialNotFound, CheckMemoReadContext(withViewer(read, owner)).Denial)
	memo.Visibility = store.Public
	memo.DeletedTs = new(int64(1))
	require.Equal(t, MemoReadDenialNotFound, CheckMemoReadContext(withViewer(read, owner)).Denial)
	memo.DeletedTs = nil
	read.CreatorValid = false
	require.Equal(t, MemoReadDenialNotFound, CheckMemoReadContext(withViewer(read, owner)).Denial)
}

func withViewer(read MemoReadContext, viewer *store.User) MemoReadContext {
	read.Viewer = viewer
	return read
}
