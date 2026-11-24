package test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/internal/testutil/legacyspace"
	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

func TestGetUserStats_TagCount(t *testing.T) {
	ctx := context.Background()

	// Create test service
	ts := NewTestService(t)
	defer ts.Cleanup()

	// Create a test host user
	user, err := ts.CreateHostUser(ctx, "test-user")
	require.NoError(t, err)

	// Create user context for authentication
	userCtx := ts.CreateUserContext(ctx, user.ID)

	// Create a memo with a single tag
	memo, err := ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "test-memo-1",
		CreatorID:  user.ID,
		Content:    "This is a test memo with #test tag",
		Visibility: store.Public,
		Payload: &storepb.MemoPayload{
			Tags: []string{"test", "test"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, memo)

	// Test GetUserStats
	userName := fmt.Sprintf("users/%s", user.Username)
	response, err := ts.Service.GetUserStats(userCtx, &apipb.GetUserStatsRequest{
		Name: userName,
	})
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, fmt.Sprintf("users/%s/stats", user.Username), response.Name)

	// A memo contributes at most once to an exact tag count, even if its payload
	// accidentally contains the same derived membership more than once.
	require.Contains(t, response.TagCounts, "test")
	require.Equal(t, int32(1), response.TagCounts["test"], "Tag count should be 1 for a single occurrence")

	// Create another memo with the same tag
	memo2, err := ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "test-memo-2",
		CreatorID:  user.ID,
		Content:    "Another memo with #test tag",
		Visibility: store.Public,
		Payload: &storepb.MemoPayload{
			Tags: []string{"test"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, memo2)

	// Test GetUserStats again
	response2, err := ts.Service.GetUserStats(userCtx, &apipb.GetUserStatsRequest{
		Name: userName,
	})
	require.NoError(t, err)
	require.NotNil(t, response2)

	// Check that the tag count is exactly 2, not 3
	require.Contains(t, response2.TagCounts, "test")
	require.Equal(t, int32(2), response2.TagCounts["test"], "Tag count should be 2 for two occurrences")

	// Test with a new unique tag
	memo3, err := ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "test-memo-3",
		CreatorID:  user.ID,
		Content:    "Memo with #unique tag",
		Visibility: store.Public,
		Payload: &storepb.MemoPayload{
			Tags: []string{"unique"},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, memo3)

	// Test GetUserStats for the new tag
	response3, err := ts.Service.GetUserStats(userCtx, &apipb.GetUserStatsRequest{
		Name: userName,
	})
	require.NoError(t, err)
	require.NotNil(t, response3)

	// Check that the unique tag count is exactly 1
	require.Contains(t, response3.TagCounts, "unique")
	require.Equal(t, int32(1), response3.TagCounts["unique"], "New tag count should be 1 for first occurrence")

	// The original test tag should still be 2
	require.Contains(t, response3.TagCounts, "test")
	require.Equal(t, int32(2), response3.TagCounts["test"], "Original tag count should remain 2")

	_, err = ts.Service.GetUserStats(userCtx, &apipb.GetUserStatsRequest{
		Name: "users/1",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "user not found")
}

func TestGetUserStats_TagCountPreservesExactIdentity(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	const (
		composedTag   = "caf\u00e9"
		decomposedTag = "cafe\u0301"
	)

	user, err := ts.CreateHostUser(ctx, "exact-tag-stats-user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	_, err = ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "exact-tag-stats-memo",
		CreatorID:  user.ID,
		Content:    "Exact tag membership fixture",
		Visibility: store.Public,
		Payload: &storepb.MemoPayload{
			Tags: []string{"book", "book/fiction", "book", "Work", "work", composedTag, decomposedTag},
		},
	})
	require.NoError(t, err)
	_, err = ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "legacy-tag-stats-memo",
		CreatorID:  user.ID,
		Content:    "Only child paths present in the old payload",
		Visibility: store.Public,
		Payload: &storepb.MemoPayload{
			Tags: []string{"book/history/local", "book/biography"},
		},
	})
	require.NoError(t, err)

	response, err := ts.Service.GetUserStats(userCtx, &apipb.GetUserStatsRequest{
		Name: fmt.Sprintf("users/%s", user.Username),
	})
	require.NoError(t, err)
	want := map[string]int32{
		"book":               2,
		"book/fiction":       1,
		"book/history":       1,
		"book/history/local": 1,
		"book/biography":     1,
		"Work":               1,
		"work":               1,
		composedTag:          1,
		decomposedTag:        1,
	}
	require.Equal(t, want, response.TagCounts)
	listed, err := ts.Service.ListUserStats(userCtx, &apipb.ListUserStatsRequest{Parent: fmt.Sprintf("users/%s", user.Username)})
	require.NoError(t, err)
	require.Len(t, listed.UserStats, 1)
	require.Equal(t, want, listed.UserStats[0].TagCounts)
}

func TestGetUserStats_MemoUpdatedTimestamps(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateHostUser(ctx, "ts-user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	memo, err := ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "ts-memo-1",
		CreatorID:  user.ID,
		Content:    "first content",
		Visibility: store.Public,
	})
	require.NoError(t, err)
	require.NotNil(t, memo)

	// SQLite UpdateMemo only sets fields explicitly passed (created_ts default
	// fires on INSERT only). So bump updated_ts explicitly to simulate an edit
	// happening after creation.
	newContent := "second content"
	newUpdatedTs := memo.UpdatedTs + 100
	require.NoError(t, ts.Store.UpdateMemo(ctx, &store.UpdateMemo{
		ID:        memo.ID,
		Content:   &newContent,
		UpdatedTs: &newUpdatedTs,
	}))

	userName := fmt.Sprintf("users/%s", user.Username)
	resp, err := ts.Service.GetUserStats(userCtx, &apipb.GetUserStatsRequest{Name: userName})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Nil(t, resp.DailyActivity)

	require.Len(t, resp.MemoCreatedTimestamps, 1, "should have one created timestamp")
	require.Len(t, resp.MemoUpdatedTimestamps, 1, "should have one updated timestamp")

	require.Equal(t, memo.CreatedTs, resp.MemoCreatedTimestamps[0].AsTime().Unix())
	require.Equal(t, newUpdatedTs, resp.MemoUpdatedTimestamps[0].AsTime().Unix())
	require.Greater(
		t,
		resp.MemoUpdatedTimestamps[0].AsTime().Unix(),
		resp.MemoCreatedTimestamps[0].AsTime().Unix(),
		"updated_ts should be after created_ts after an edit",
	)
}

func TestGetUserStats_DailyActivity(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateHostUser(ctx, "daily-stats-user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)
	request := &apipb.GetUserStatsRequest{Name: "users/" + user.Username, ActivityByDay: true}

	empty, err := ts.Service.GetUserStats(userCtx, request)
	require.NoError(t, err)
	require.NotNil(t, empty.DailyActivity)
	require.Empty(t, empty.DailyActivity.CreatedCounts)
	require.Empty(t, empty.DailyActivity.UpdatedCounts)

	// Shanghai midnight is 16:00 UTC on the preceding date.
	midnight := time.Date(2026, time.May, 1, 16, 0, 0, 0, time.UTC)
	for index, created := range []time.Time{midnight.Add(-time.Second), midnight, midnight.Add(time.Minute), midnight.AddDate(0, 0, 1)} {
		memo, err := ts.Store.CreateMemo(ctx, &store.Memo{
			UID:        fmt.Sprintf("daily-stats-memo-%d", index),
			CreatorID:  user.ID,
			Visibility: store.Private,
			CreatedTs:  created.Unix(),
			UpdatedTs:  midnight.AddDate(0, 0, 2).Unix(),
			Payload:    &storepb.MemoPayload{Tags: []string{"work/project"}},
		})
		require.NoError(t, err)
		if index == 3 {
			archived := store.Archived
			require.NoError(t, ts.Store.UpdateMemo(ctx, &store.UpdateMemo{ID: memo.ID, RowStatus: &archived}))
		}
	}

	stats, err := ts.Service.GetUserStats(userCtx, request)
	require.NoError(t, err)
	require.NotNil(t, stats.DailyActivity)
	require.EqualValues(t, 3, stats.TotalMemoCount)
	require.Equal(t, map[string]int32{"work": 3, "work/project": 3}, stats.TagCounts)
	require.Equal(t, map[string]int32{"2026-05-01": 1, "2026-05-02": 2}, stats.DailyActivity.CreatedCounts)
	require.Equal(t, map[string]int32{"2026-05-04": 3}, stats.DailyActivity.UpdatedCounts)
	require.Empty(t, stats.MemoCreatedTimestamps)
	require.Empty(t, stats.MemoUpdatedTimestamps)

	archives, err := ts.Service.ListUserStats(userCtx, &apipb.ListUserStatsRequest{Parent: request.Name, State: apipb.State_ARCHIVED})
	require.NoError(t, err)
	require.Len(t, archives.UserStats, 1)
	require.Len(t, archives.UserStats[0].MemoCreatedTimestamps, 1)
	require.Nil(t, archives.UserStats[0].DailyActivity)
}

func TestGetUserStats_PinnedMemoUsesCanonicalResourceName(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateHostUser(ctx, "pinned-stats-user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)
	memo, err := ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "pinned-stats-memo",
		CreatorID:  user.ID,
		Content:    "pinned",
		Visibility: store.Public,
	})
	require.NoError(t, err)
	pinned := true
	require.NoError(t, ts.Store.UpdateMemo(ctx, &store.UpdateMemo{ID: memo.ID, Pinned: &pinned}))

	resp, err := ts.Service.GetUserStats(userCtx, &apipb.GetUserStatsRequest{Name: fmt.Sprintf("users/%s", user.Username)})
	require.NoError(t, err)
	require.Equal(t, []string{"memos/pinned-stats-memo"}, resp.PinnedMemos)
}

func TestListUserStats_FilterExcludesPrivateMemos(t *testing.T) {
	ctx := context.Background()

	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateHostUser(ctx, "stats-filter-user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	_, err = ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "stats-filter-public",
		CreatorID:  user.ID,
		Content:    "public memo",
		Visibility: store.Public,
		Payload:    &storepb.MemoPayload{Tags: []string{"public", "public"}},
	})
	require.NoError(t, err)
	_, err = ts.Store.CreateMemo(ctx, &store.Memo{
		UID:        "stats-filter-private",
		CreatorID:  user.ID,
		Content:    "private memo",
		Visibility: store.Private,
		Payload:    &storepb.MemoPayload{Tags: []string{"private", "private"}},
	})
	require.NoError(t, err)

	unfilteredResp, err := ts.Service.ListUserStats(userCtx, &apipb.ListUserStatsRequest{Parent: "users/-"})
	require.NoError(t, err)
	require.Len(t, unfilteredResp.UserStats, 1)
	require.Equal(t, int32(1), unfilteredResp.UserStats[0].TagCounts["public"])
	require.Equal(t, int32(1), unfilteredResp.UserStats[0].TagCounts["private"])

	filteredResp, err := ts.Service.ListUserStats(userCtx, &apipb.ListUserStatsRequest{
		Parent: "users/-",
		Filter: `visibility in ["PUBLIC", "PROTECTED"]`,
	})
	require.NoError(t, err)
	require.Len(t, filteredResp.UserStats, 1)
	require.Equal(t, int32(1), filteredResp.UserStats[0].TagCounts["public"])
	require.NotContains(t, filteredResp.UserStats[0].TagCounts, "private")
}

func TestUserStatsUseMemoLocalAccess(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	owner, err := ts.CreateRegularUser(ctx, "stats-access-owner")
	require.NoError(t, err)
	member, err := ts.CreateRegularUser(ctx, "stats-access-member")
	require.NoError(t, err)
	applicationAdmin, err := ts.CreateHostUser(ctx, "stats-access-app-admin")
	require.NoError(t, err)

	db := ts.Store.GetDriver().GetDB()
	spaceID, err := legacyspace.Seed(ctx, db, "stats-access-space", "Stats access", owner.ID)
	require.NoError(t, err)
	require.NoError(t, legacyspace.AddMember(ctx, db, spaceID, member.ID, "USER"))

	fixtures := []struct {
		uid        string
		visibility store.Visibility
		tag        string
	}{
		{uid: "stats-access-public", visibility: store.Public, tag: "public"},
		{uid: "stats-access-protected", visibility: store.Protected, tag: "protected"},
		{uid: "stats-access-private", visibility: store.Private, tag: "private"},
	}
	for _, fixture := range fixtures {
		_, err := ts.Store.CreateMemo(ctx, &store.Memo{
			UID:        fixture.uid,
			CreatorID:  owner.ID,
			Content:    fixture.tag,
			Visibility: fixture.visibility,
			Payload:    &storepb.MemoPayload{Tags: []string{fixture.tag}},
		})
		require.NoError(t, err)
	}
	legacyMemo, err := ts.Store.CreateMemo(ctx, &store.Memo{
		UID: "stats-access-members", CreatorID: owner.ID, Content: "space", Visibility: store.Private,
		Payload: &storepb.MemoPayload{Tags: []string{"space"}},
	})
	require.NoError(t, err)
	require.NoError(t, legacyspace.PlaceMemo(ctx, db, legacyMemo.ID, spaceID, "SPACE"))

	ownerName := fmt.Sprintf("users/%s", owner.Username)
	tests := []struct {
		name       string
		requestCtx context.Context
		wantTags   []string
		denyTags   []string
	}{
		{
			name:       "owner",
			requestCtx: ts.CreateUserContext(ctx, owner.ID),
			wantTags:   []string{"public", "protected", "private", "space"},
		},
		{
			name:       "space member",
			requestCtx: ts.CreateUserContext(ctx, member.ID),
			denyTags:   []string{"public", "protected", "private", "space"},
		},
		{
			name:       "nonmember application admin",
			requestCtx: ts.CreateUserContext(ctx, applicationAdmin.ID),
			denyTags:   []string{"public", "protected", "private", "space"},
		},
		{
			name:       "anonymous",
			requestCtx: ctx,
			denyTags:   []string{"public", "protected", "private", "space"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := ts.Service.GetUserStats(test.requestCtx, &apipb.GetUserStatsRequest{Name: ownerName})
			require.NoError(t, err)
			require.Equal(t, int32(len(test.wantTags)), response.TotalMemoCount)
			for _, tag := range test.wantTags {
				require.Equal(t, int32(1), response.TagCounts[tag], "expected tag %q to be visible", tag)
			}
			for _, tag := range test.denyTags {
				require.NotContains(t, response.TagCounts, tag)
			}
		})
	}

	memberStats, err := ts.Service.ListUserStats(ts.CreateUserContext(ctx, member.ID), &apipb.ListUserStatsRequest{Parent: "users/-"})
	require.NoError(t, err)
	require.Empty(t, memberStats.UserStats)
}

func TestGetUserStats_AttachmentStorage(t *testing.T) {
	ts := NewTestService(t)
	defer ts.Cleanup()
	ctx := context.Background()
	owner, err := ts.CreateRegularUser(ctx, "storage_owner")
	require.NoError(t, err)
	other, err := ts.CreateRegularUser(ctx, "storage_other")
	require.NoError(t, err)
	ownerCtx := ts.CreateUserContext(ctx, owner.ID)
	request := &apipb.GetUserStatsRequest{Name: "users/" + owner.Username}

	stats, err := ts.Service.GetUserStats(ownerCtx, request)
	require.NoError(t, err)
	require.NotNil(t, stats.AttachmentStorageBytes)
	require.Zero(t, *stats.AttachmentStorageBytes)

	attachment := createTestImageAttachment(ownerCtx, t, ts, "owner.png")
	createTestImageAttachment(ts.CreateUserContext(ctx, other.ID), t, ts, "other.png")

	// Memo filtering must not turn account storage into a partial total.
	stats, err = ts.Service.GetUserStats(ownerCtx, &apipb.GetUserStatsRequest{Name: request.Name, Filter: `content.contains("no matches")`})
	require.NoError(t, err)
	require.Zero(t, stats.TotalMemoCount)
	require.NotNil(t, stats.AttachmentStorageBytes)
	require.Equal(t, attachment.Size, *stats.AttachmentStorageBytes)

	for _, viewerCtx := range []context.Context{ctx, ts.CreateUserContext(ctx, other.ID)} {
		stats, err = ts.Service.GetUserStats(viewerCtx, request)
		require.NoError(t, err)
		require.Nil(t, stats.AttachmentStorageBytes, "private storage totals must not appear in public user stats")
	}

	_, err = ts.Service.DeleteAttachment(ownerCtx, &apipb.DeleteAttachmentRequest{Name: attachment.Name})
	require.NoError(t, err)
	stats, err = ts.Service.GetUserStats(ownerCtx, request)
	require.NoError(t, err)
	require.NotNil(t, stats.AttachmentStorageBytes)
	require.Zero(t, *stats.AttachmentStorageBytes)
}

func TestListUserStatsParent(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	alice, err := ts.CreateRegularUser(ctx, "stats-alice")
	require.NoError(t, err)
	bob, err := ts.CreateRegularUser(ctx, "stats-bob")
	require.NoError(t, err)
	for _, user := range []*store.User{alice, bob} {
		_, err := ts.Service.CreateMemo(ts.CreateUserContext(ctx, user.ID), &apipb.CreateMemoRequest{
			Memo: &apipb.Memo{Content: "public memo", Visibility: apipb.Visibility_PUBLIC},
		})
		require.NoError(t, err)
	}
	aliceCtx := ts.CreateUserContext(ctx, alice.ID)

	all, err := ts.Service.ListUserStats(aliceCtx, &apipb.ListUserStatsRequest{Parent: "users/-"})
	require.NoError(t, err)
	require.Len(t, all.UserStats, 1)
	require.Equal(t, "users/stats-alice/stats", all.UserStats[0].Name)

	one, err := ts.Service.ListUserStats(aliceCtx, &apipb.ListUserStatsRequest{Parent: "users/stats-bob"})
	require.NoError(t, err)
	require.Empty(t, one.UserStats)

	_, err = ts.Service.ListUserStats(aliceCtx, &apipb.ListUserStatsRequest{Parent: "users/missing"})
	require.Equal(t, codes.NotFound, status.Code(err))

	_, err = ts.Service.ListUserStats(aliceCtx, &apipb.ListUserStatsRequest{})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}
