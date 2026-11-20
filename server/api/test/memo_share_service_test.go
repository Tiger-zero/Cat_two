package test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/store"
)

func TestDeleteMemoShare_VerifiesShareBelongsToMemo(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	userOne, err := ts.CreateRegularUser(ctx, "share-owner-one")
	require.NoError(t, err)
	userTwo, err := ts.CreateRegularUser(ctx, "share-owner-two")
	require.NoError(t, err)

	userOneCtx := ts.CreateUserContext(ctx, userOne.ID)
	userTwoCtx := ts.CreateUserContext(ctx, userTwo.ID)

	memoOne, err := ts.Service.CreateMemo(userOneCtx, &apipb.CreateMemoRequest{
		Memo: &apipb.Memo{
			Content:    "memo one",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	memoTwo, err := ts.Service.CreateMemo(userTwoCtx, &apipb.CreateMemoRequest{
		Memo: &apipb.Memo{
			Content:    "memo two",
			Visibility: apipb.Visibility_PRIVATE,
		},
	})
	require.NoError(t, err)

	const shareToken = "legacy-share-two"
	seedLegacyMemoShare(ctx, t, ts, memoTwo.Name, userTwo.ID, shareToken, nil)
	forgedName := memoOne.Name + "/shares/" + shareToken

	_, err = ts.Service.DeleteMemoShare(userOneCtx, &apipb.DeleteMemoShareRequest{
		Name: forgedName,
	})
	require.Error(t, err)
	require.Equal(t, codes.NotFound, status.Code(err))

	requireSharePresent(ctx, t, ts, shareToken)
}

func TestDeleteMemoShare_RevalidatesSpaceWriteAuthority(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "share-space-owner")
	require.NoError(t, err)
	admin, err := ts.CreateRegularUser(ctx, "share-space-admin")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	space, err := ts.Store.CreateSpace(ctx, &store.Space{UID: "share-space", Title: "Share Space"}, owner.ID)
	require.NoError(t, err)
	_, err = ts.InviteAndAcceptSpaceMember(ctx, &store.SpaceMember{
		SpaceID: space.ID, UserID: admin.ID, Role: store.SpaceMemberRoleAdmin,
	}, owner.ID)
	require.NoError(t, err)
	memo, err := ts.Service.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{Memo: &apipb.Memo{
		Content: "assigned share", Space: new("spaces/" + space.UID),
	}})
	require.NoError(t, err)
	const shareToken = "legacy-assigned-share"
	shareName := seedLegacyMemoShare(ctx, t, ts, memo.Name, owner.ID, shareToken, nil)

	require.NoError(t, ts.Store.DeleteSpaceMember(ctx, &store.DeleteSpaceMember{SpaceID: space.ID, UserID: owner.ID}, admin.ID))
	_, err = ts.Service.DeleteMemoShare(ownerCtx, &apipb.DeleteMemoShareRequest{Name: shareName})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	requireSharePresent(ctx, t, ts, shareToken)

	_, err = ts.InviteAndAcceptSpaceMember(ctx, &store.SpaceMember{
		SpaceID: space.ID, UserID: owner.ID, Role: store.SpaceMemberRoleUser,
	}, admin.ID)
	require.NoError(t, err)
	_, err = ts.Service.DeleteMemoShare(ownerCtx, &apipb.DeleteMemoShareRequest{Name: shareName})
	require.NoError(t, err)
	storedShare, err := ts.Store.GetMemoShare(ctx, &store.FindMemoShare{UID: new(shareToken)})
	require.NoError(t, err)
	require.Nil(t, storedShare)
}

func TestDeleteMemoShare_OwnerCanRevokeLegacyAdminShare(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "legacy-share-owner")
	require.NoError(t, err)
	legacyAdmin, err := ts.CreateHostUser(ctx, "legacy-share-admin")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	memo, err := ts.Service.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{Memo: &apipb.Memo{
		Content:    "memo with a legacy admin share",
		Visibility: apipb.Visibility_PRIVATE,
	}})
	require.NoError(t, err)
	const shareToken = "legacy-admin-created-share"
	shareName := seedLegacyMemoShare(ctx, t, ts, memo.Name, legacyAdmin.ID, shareToken, nil)
	adminCtx := ts.CreateUserContext(ctx, legacyAdmin.ID)
	_, err = ts.Service.ListMemoShares(adminCtx, &apipb.ListMemoSharesRequest{Parent: memo.Name})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = ts.Service.DeleteMemoShare(adminCtx, &apipb.DeleteMemoShareRequest{Name: shareName})
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	shares, err := ts.Service.ListMemoShares(ownerCtx, &apipb.ListMemoSharesRequest{Parent: memo.Name})
	require.NoError(t, err)
	require.Len(t, shares.MemoShares, 1)
	require.Equal(t, shareName, shares.MemoShares[0].Name)
	_, err = ts.Service.DeleteMemoShare(ownerCtx, &apipb.DeleteMemoShareRequest{Name: shareName})
	require.NoError(t, err)

	share, err := ts.Store.GetMemoShare(ctx, &store.FindMemoShare{UID: new(shareToken)})
	require.NoError(t, err)
	require.Nil(t, share)
	_, err = ts.Service.GetSharedMemo(ctx, &apipb.GetSharedMemoRequest{ShareToken: shareToken})
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestCreateMemoShare_DisabledForMemoOwner(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "share-disabled-owner")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	memo, err := ts.Service.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{
		Memo: &apipb.Memo{Content: "this note belongs to its author"},
	})
	require.NoError(t, err)

	share, err := ts.Service.CreateMemoShare(ownerCtx, &apipb.CreateMemoShareRequest{Parent: memo.Name, MemoShare: &apipb.MemoShare{}})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	require.Nil(t, share)
	shares, err := ts.Service.ListMemoShares(ownerCtx, &apipb.ListMemoSharesRequest{Parent: memo.Name})
	require.NoError(t, err)
	require.Empty(t, shares.MemoShares)
}

func TestGetSharedMemo_LegacyTokensNeverGrantAccess(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "legacy-token-owner")
	require.NoError(t, err)
	other, err := ts.CreateRegularUser(ctx, "legacy-token-other")
	require.NoError(t, err)
	admin, err := ts.CreateHostUser(ctx, "legacy-token-admin")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)

	parent, err := ts.Service.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{
		Memo: &apipb.Memo{Content: "parent is author-only"},
	})
	require.NoError(t, err)
	comment, err := ts.Service.CreateMemoComment(ownerCtx, &apipb.CreateMemoCommentRequest{
		Parent: parent.Name, Comment: &apipb.Memo{Content: "comment is author-only"},
	})
	require.NoError(t, err)
	visibleComment, err := ts.Service.GetMemo(ownerCtx, &apipb.GetMemoRequest{Name: comment.Name})
	require.NoError(t, err)
	require.Equal(t, parent.Name, visibleComment.GetParent())
	require.NotEmpty(t, visibleComment.Relations)

	reaction, err := ts.Service.UpsertMemoReaction(ownerCtx, &apipb.UpsertMemoReactionRequest{
		Parent: parent.Name, Reaction: &apipb.Reaction{ReactionType: "👍"},
	})
	require.NoError(t, err)
	visibleParent, err := ts.Service.GetMemo(ownerCtx, &apipb.GetMemoRequest{Name: parent.Name})
	require.NoError(t, err)
	require.Len(t, visibleParent.Reactions, 1)
	require.Equal(t, reaction.Name, visibleParent.Reactions[0].Name)

	archived, err := ts.Service.CreateMemo(ownerCtx, &apipb.CreateMemoRequest{
		Memo: &apipb.Memo{Content: "archived note is author-only"},
	})
	require.NoError(t, err)
	archivedID := parseMemoIDFromNameForTest(t, ts, archived.Name)
	require.NoError(t, ts.Store.UpdateMemo(ctx, &store.UpdateMemo{ID: archivedID, RowStatus: new(store.Archived)}))

	seedLegacyMemoShare(ctx, t, ts, parent.Name, owner.ID, "active-legacy-token", nil)
	seedLegacyMemoShare(ctx, t, ts, parent.Name, owner.ID, "expired-legacy-token", new(time.Now().Add(-time.Hour).Unix()))
	seedLegacyMemoShare(ctx, t, ts, comment.Name, owner.ID, "comment-legacy-token", nil)
	seedLegacyMemoShare(ctx, t, ts, archived.Name, owner.ID, "archived-legacy-token", nil)
	for _, token := range []string{"active-legacy-token", "expired-legacy-token", "comment-legacy-token", "archived-legacy-token", "missing-token"} {
		t.Run(token, func(t *testing.T) {
			for _, actor := range []context.Context{ctx, ownerCtx, ts.CreateUserContext(ctx, other.ID), ts.CreateUserContext(ctx, admin.ID)} {
				memo, err := ts.Service.GetSharedMemo(actor, &apipb.GetSharedMemoRequest{ShareToken: token})
				require.Equal(t, codes.NotFound, status.Code(err))
				require.Nil(t, memo)
			}
		})
	}
}

func seedLegacyMemoShare(ctx context.Context, t *testing.T, ts *TestService, memoName string, creatorID int32, token string, expiresTs *int64) string {
	t.Helper()
	_, err := ts.Store.CreateMemoShare(ctx, &store.MemoShare{
		UID: token, MemoID: parseMemoIDFromNameForTest(t, ts, memoName), CreatorID: creatorID, ExpiresTs: expiresTs,
	})
	require.NoError(t, err)
	return memoName + "/shares/" + token
}

func parseMemoIDFromNameForTest(t *testing.T, ts *TestService, memoName string) int32 {
	t.Helper()

	memoUID, ok := strings.CutPrefix(memoName, "memos/")
	require.True(t, ok, "memo name must start with memos/: %s", memoName)

	memo, err := ts.Store.GetMemo(context.Background(), &store.FindMemo{UID: &memoUID})
	require.NoError(t, err)
	require.NotNil(t, memo)

	return memo.ID
}

func requireSharePresent(ctx context.Context, t *testing.T, ts *TestService, token string) {
	t.Helper()
	share, err := ts.Store.GetMemoShare(ctx, &store.FindMemoShare{UID: &token})
	require.NoError(t, err)
	require.NotNil(t, share)
}
