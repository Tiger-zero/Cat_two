package test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/store"
)

type usageLimitEntry = apipb.InstanceSetting_UsageLimitsSetting_UsageLimit

const usageLimitsSettingName = "instance/settings/USAGE_LIMITS"

func usageLimitsSetting(limits ...*usageLimitEntry) *apipb.InstanceSetting {
	return &apipb.InstanceSetting{
		Name:  usageLimitsSettingName,
		Value: &apipb.InstanceSetting_UsageLimitsSetting_{UsageLimitsSetting: &apipb.InstanceSetting_UsageLimitsSetting{Limits: limits}},
	}
}

// seedLegacyUsageLimits stores a quota row from an older Cat_two release.
func seedLegacyUsageLimits(ctx context.Context, t *testing.T, ts *TestService, limits ...*storepb.InstanceUsageLimitsSetting_UsageLimit) {
	t.Helper()
	_, err := ts.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_USAGE_LIMITS,
		Value: &storepb.InstanceSetting_UsageLimitsSetting{
			UsageLimitsSetting: &storepb.InstanceUsageLimitsSetting{Limits: limits},
		},
	})
	require.NoError(t, err)
}

func TestUsageLimitsSettingPermissions(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	admin, err := ts.CreateHostUser(ctx, "limits-admin")
	require.NoError(t, err)
	user, err := ts.CreateRegularUser(ctx, "limits-user")
	require.NoError(t, err)
	adminCtx := ts.CreateUserContext(ctx, admin.ID)

	for _, caller := range []struct {
		ctx  context.Context
		code codes.Code
	}{
		{ctx: ts.CreateUserContext(ctx, user.ID), code: codes.PermissionDenied},
		{ctx: ctx, code: codes.Unauthenticated},
	} {
		_, err := ts.Service.GetInstanceSetting(caller.ctx, &apipb.GetInstanceSettingRequest{Name: usageLimitsSettingName})
		require.Equal(t, caller.code, status.Code(err), "the entries name users, so only admins read them")
		_, err = ts.Service.UpdateInstanceSetting(caller.ctx, &apipb.UpdateInstanceSettingRequest{InstanceSetting: usageLimitsSetting(&usageLimitEntry{AllUsers: true})})
		require.Equal(t, caller.code, status.Code(err))
	}

	setting, err := ts.Service.GetInstanceSetting(adminCtx, &apipb.GetInstanceSettingRequest{Name: usageLimitsSettingName})
	require.NoError(t, err)
	limits := setting.GetUsageLimitsSetting().Limits
	require.Len(t, limits, 1, "the built-in default is the only entry")
	require.True(t, limits[0].AllUsers)
	require.Equal(t, store.DefaultUploadBytes, limits[0].GetUploadBytes())
	require.Equal(t, store.DefaultMemoContentBytes, limits[0].GetMemoContentBytes())
	require.Nil(t, limits[0].MemoCount)
}

func TestLegacyUsageLimitsSettingIsReadOnly(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	admin, err := ts.CreateHostUser(ctx, "crud-admin")
	require.NoError(t, err)
	alice, err := ts.CreateRegularUser(ctx, "crud-alice")
	require.NoError(t, err)
	bob, err := ts.CreateRegularUser(ctx, "crud-bob")
	require.NoError(t, err)
	adminCtx := ts.CreateUserContext(ctx, admin.ID)
	aliceName, bobName := api.BuildUserName(alice.Username), api.BuildUserName(bob.Username)

	seedLegacyUsageLimits(ctx, t, ts,
		&storepb.InstanceUsageLimitsSetting_UsageLimit{UserIds: []int32{alice.ID, bob.ID}, StorageBytes: new(int64(5 << 30))},
		&storepb.InstanceUsageLimitsSetting_UsageLimit{UserIds: []int32{alice.ID}, MemoCount: new(int64(0))},
		&storepb.InstanceUsageLimitsSetting_UsageLimit{AllUsers: true, StorageBytes: new(int64(1 << 30)), MemoCount: new(int64(1000))},
	)
	got, err := ts.Service.GetInstanceSetting(adminCtx, &apipb.GetInstanceSettingRequest{Name: usageLimitsSettingName})
	require.NoError(t, err)
	saved := got.GetUsageLimitsSetting()
	require.Len(t, saved.Limits, 3)
	require.Equal(t, []string{aliceName, bobName}, saved.Limits[0].Users)
	require.Equal(t, int64(5<<30), saved.Limits[0].GetStorageBytes())
	require.Nil(t, saved.Limits[0].MemoCount, "unset stays unset")
	require.Zero(t, saved.Limits[1].GetMemoCount())
	require.NotNil(t, saved.Limits[1].MemoCount, "an explicit zero stays explicit")

	for _, name := range []string{aliceName, api.BuildUserName(admin.Username)} {
		usage, err := ts.Service.GetUserUsage(adminCtx, &apipb.GetUserUsageRequest{Name: name + "/usage"})
		require.NoError(t, err)
		require.EqualValues(t, -1, usage.LimitIndex, "stored entries apply to neither regular users nor admins")
		require.Nil(t, usage.Limits.StorageBytes)
		require.Nil(t, usage.Limits.MemoCount)
	}

	_, err = ts.Service.UpdateInstanceSetting(adminCtx, &apipb.UpdateInstanceSettingRequest{
		InstanceSetting: usageLimitsSetting(&usageLimitEntry{AllUsers: true, StorageBytes: new(int64(1))}),
	})
	require.Equal(t, codes.Unimplemented, status.Code(err))

	got, err = ts.Service.GetInstanceSetting(adminCtx, &apipb.GetInstanceSettingRequest{Name: usageLimitsSettingName})
	require.NoError(t, err)
	require.Equal(t, saved.String(), got.GetUsageLimitsSetting().String(), "a refused update leaves the limits unchanged")

	// Renaming a user keeps their entries; deleting the user removes them.
	renamed := "crud-alice-renamed"
	_, err = ts.Store.UpdateUser(ctx, &store.UpdateUser{ID: alice.ID, Username: &renamed})
	require.NoError(t, err)
	got, err = ts.Service.GetInstanceSetting(adminCtx, &apipb.GetInstanceSettingRequest{Name: usageLimitsSettingName})
	require.NoError(t, err)
	require.Equal(t, []string{api.BuildUserName(renamed), bobName}, got.GetUsageLimitsSetting().Limits[0].Users)
	_, err = ts.Store.DeleteUser(ctx, &store.DeleteUser{ID: alice.ID})
	require.NoError(t, err)
	got, err = ts.Service.GetInstanceSetting(adminCtx, &apipb.GetInstanceSettingRequest{Name: usageLimitsSettingName})
	require.NoError(t, err)
	require.Len(t, got.GetUsageLimitsSetting().Limits, 2)
	require.Equal(t, []string{bobName}, got.GetUsageLimitsSetting().Limits[0].Users)
	require.True(t, got.GetUsageLimitsSetting().Limits[1].AllUsers)
}

func TestStoredUsageLimitsDoNotRestrictMemos(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	defer ts.Service.CloseUploads()
	user, err := ts.CreateRegularUser(ctx, "memo-user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)
	subject := api.BuildUserName(user.Username)

	seedLegacyUsageLimits(ctx, t, ts,
		&storepb.InstanceUsageLimitsSetting_UsageLimit{AllUsers: true, MemoCount: new(int64(1)), MemoContentBytes: new(int64(6))},
	)

	content := strings.Repeat("a", 8193)
	first, err := ts.Service.CreateMemo(userCtx, &apipb.CreateMemoRequest{Memo: &apipb.Memo{Content: content}})
	require.NoError(t, err)
	require.Equal(t, content, first.Content)

	second, err := ts.Service.CreateMemo(userCtx, &apipb.CreateMemoRequest{Memo: &apipb.Memo{Content: "second"}})
	require.NoError(t, err)
	second, err = ts.Service.UpdateMemo(userCtx, &apipb.UpdateMemoRequest{
		Memo:       &apipb.Memo{Name: second.Name, Content: content},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"content"}},
	})
	require.NoError(t, err)
	require.Equal(t, content, second.Content)

	_, err = ts.Service.CreateMemoComment(userCtx, &apipb.CreateMemoCommentRequest{
		Parent: first.Name, Comment: &apipb.Memo{Content: content},
	})
	require.NoError(t, err)

	archive := buildImportArchive(t, user.Username, nil)
	response, err := ts.Service.ImportMemos(userCtx, &apipb.ImportMemosRequest{
		Name: subject, Upload: &apipb.ImportMemosRequest_Spec{Spec: &apipb.ImportMemosSpec{TotalSize: int64(len(archive))}},
		Data: archive, FinishWrite: true,
	})
	require.NoError(t, err)
	report := response.GetReport()
	require.NotNil(t, report)
	require.Zero(t, report.Failed)
	require.EqualValues(t, 1, report.Created)

	usage, err := ts.Service.GetUserUsage(userCtx, &apipb.GetUserUsageRequest{Name: subject + "/usage"})
	require.NoError(t, err)
	require.EqualValues(t, 4, usage.MemoCount)
	require.EqualValues(t, -1, usage.LimitIndex)
	require.Nil(t, usage.Limits.MemoCount)
	require.Nil(t, usage.Limits.MemoContentBytes)
}

func TestStoredUsageLimitsDoNotRestrictAttachmentWrites(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	defer ts.Service.CloseUploads()
	user, err := ts.CreateRegularUser(ctx, "upload-user")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)
	subject := api.BuildUserName(user.Username)

	seedLegacyUsageLimits(ctx, t, ts,
		&storepb.InstanceUsageLimitsSetting_UsageLimit{AllUsers: true, StorageBytes: new(int64(1)), UploadBytes: new(int64(1))},
	)

	first, err := ts.Service.CreateAttachment(userCtx, &apipb.CreateAttachmentRequest{
		Attachment: &apipb.Attachment{Filename: "one.bin", Type: "application/octet-stream", Content: []byte("abcd")},
	})
	require.NoError(t, err)
	require.EqualValues(t, 4, first.Size)

	started, err := ts.Service.UploadAttachment(userCtx, &apipb.UploadAttachmentRequest{
		Upload: &apipb.UploadAttachmentRequest_Spec{Spec: &apipb.UploadAttachmentSpec{
			Attachment: &apipb.Attachment{Filename: "two.bin", Type: "application/octet-stream"}, TotalSize: 5,
		}},
	})
	require.NoError(t, err)
	uploaded, err := ts.Service.UploadAttachment(userCtx, &apipb.UploadAttachmentRequest{
		Upload: &apipb.UploadAttachmentRequest_UploadId{UploadId: started.UploadId},
		Data:   []byte("abcde"), FinishWrite: true,
	})
	require.NoError(t, err)
	require.EqualValues(t, 5, uploaded.Attachment.Size)

	replaced, err := ts.Service.UploadAttachment(userCtx, &apipb.UploadAttachmentRequest{
		Upload: &apipb.UploadAttachmentRequest_Spec{Spec: &apipb.UploadAttachmentSpec{
			Attachment: &apipb.Attachment{Filename: "one.bin", Type: "application/octet-stream"},
			TotalSize:  6, ReplaceAttachment: first.Name,
		}},
		Data: []byte("abcdef"), FinishWrite: true,
	})
	require.NoError(t, err)
	require.Equal(t, first.Name, replaced.Attachment.Name)
	require.EqualValues(t, 6, replaced.Attachment.Size)

	archive := buildImportArchive(t, user.Username, []struct {
		uid, filename string
		content       []byte
	}{{"importfile1", "three.bin", []byte("abcdefg")}})
	response, err := ts.Service.ImportMemos(userCtx, &apipb.ImportMemosRequest{
		Name: subject, Upload: &apipb.ImportMemosRequest_Spec{Spec: &apipb.ImportMemosSpec{TotalSize: int64(len(archive))}},
		Data: archive, FinishWrite: true,
	})
	require.NoError(t, err)
	report := response.GetReport()
	require.NotNil(t, report)
	require.Zero(t, report.Failed)
	require.EqualValues(t, 1, report.Created)

	usage, err := ts.Service.GetUserUsage(userCtx, &apipb.GetUserUsageRequest{Name: subject + "/usage"})
	require.NoError(t, err)
	require.EqualValues(t, 18, usage.StorageBytes)
	require.EqualValues(t, -1, usage.LimitIndex)
	require.Nil(t, usage.Limits.StorageBytes)
	require.Nil(t, usage.Limits.UploadBytes)
}

func TestGetUserUsagePermissions(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()
	admin, err := ts.CreateHostUser(ctx, "usage-admin")
	require.NoError(t, err)
	alice, err := ts.CreateRegularUser(ctx, "usage-alice")
	require.NoError(t, err)
	bob, err := ts.CreateRegularUser(ctx, "usage-bob")
	require.NoError(t, err)
	aliceUsage := api.BuildUserName(alice.Username) + "/usage"

	_, err = ts.Service.GetUserUsage(ts.CreateUserContext(ctx, alice.ID), &apipb.GetUserUsageRequest{Name: aliceUsage})
	require.NoError(t, err)
	usage, err := ts.Service.GetUserUsage(ts.CreateUserContext(ctx, admin.ID), &apipb.GetUserUsageRequest{Name: aliceUsage})
	require.NoError(t, err)
	require.Equal(t, aliceUsage, usage.Name)
	_, err = ts.Service.GetUserUsage(ts.CreateUserContext(ctx, bob.ID), &apipb.GetUserUsageRequest{Name: aliceUsage})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = ts.Service.GetUserUsage(ts.CreateUserContext(ctx, bob.ID), &apipb.GetUserUsageRequest{Name: "users/nobody/usage"})
	require.Equal(t, codes.PermissionDenied, status.Code(err), "non-admins cannot probe for usernames")
	_, err = ts.Service.GetUserUsage(ts.CreateUserContext(ctx, admin.ID), &apipb.GetUserUsageRequest{Name: "users/nobody/usage"})
	require.Equal(t, codes.NotFound, status.Code(err))
	_, err = ts.Service.GetUserUsage(ctx, &apipb.GetUserUsageRequest{Name: aliceUsage})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	_, err = ts.Service.GetUserUsage(ts.CreateUserContext(ctx, alice.ID), &apipb.GetUserUsageRequest{Name: api.BuildUserName(alice.Username)})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}
