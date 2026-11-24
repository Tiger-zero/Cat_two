package test

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/proto/gen/api/apiconnect"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

func TestPersonalStorageSettingIsLocalAndReadOnly(t *testing.T) {
	ctx := context.Background()
	inst := bootInstance(ctx, t, instanceOptions{personal: true})
	inst.createAdmin(t)
	token := inst.signIn(t)

	const name = "instance/settings/STORAGE"
	request := map[string]string{"name": name}
	status, body := inst.connect(t, apiconnect.InstanceServiceGetInstanceSettingProcedure, "", request)
	require.Equal(t, http.StatusUnauthorized, status, "%s", body)
	status, body = inst.connect(t, apiconnect.InstanceServiceGetInstanceSettingProcedure, token, request)
	require.Equal(t, http.StatusOK, status, "%s", body)
	var got apipb.InstanceSetting
	require.NoError(t, protojson.Unmarshal(body, &got))
	setting := got.GetStorageSetting()
	require.NotNil(t, setting)
	require.Equal(t, apipb.InstanceSetting_StorageSetting_LOCAL, setting.GetStorageType())
	require.Equal(t, "local", setting.GetDefaultStorageId())
	require.Equal(t, store.AttachmentDirectory+"/{timestamp}_{uuid}_{filename}", setting.GetFilepathTemplate())
	require.Len(t, setting.GetStorages(), 1)
	require.Nil(t, setting.GetS3Config())

	for _, update := range []map[string]string{
		{"storageType": "DATABASE"},
		{"storageType": "LOCAL", "filepathTemplate": "../alternative/{filename}"},
	} {
		status, body = inst.connect(t, apiconnect.InstanceServiceUpdateInstanceSettingProcedure, token, map[string]any{
			"instanceSetting": map[string]any{"name": name, "storageSetting": update},
		})
		require.Equal(t, http.StatusBadRequest, status, "%s", body)
		require.Contains(t, string(body), "attachment storage is managed by Cat_two")
	}
	const attachmentID = "storage-layout"
	status, body = inst.connect(t, apiconnect.AttachmentServiceCreateAttachmentProcedure, token, map[string]any{
		"attachmentId": attachmentID,
		"attachment": map[string]any{
			"filename": "storage.txt", "content": []byte("Stored locally."),
		},
	})
	require.Equal(t, http.StatusOK, status, "%s", body)
	attachment, err := inst.server.Store.GetAttachment(ctx, &store.FindAttachment{UID: new(attachmentID), GetBlob: true})
	require.NoError(t, err)
	require.NotNil(t, attachment)
	require.Equal(t, storepb.AttachmentStorageType_LOCAL, attachment.StorageType)
	require.Nil(t, attachment.Blob)
	require.Regexp(t, `^assets/[0-9]+_[a-f0-9-]+_storage\.txt$`, attachment.Reference)
	require.FileExists(t, filepath.Join(inst.profile.Data, attachment.Reference))

	stored, err := inst.server.Store.GetUncachedInstanceSetting(ctx, &store.FindInstanceSetting{Name: "STORAGE"})
	require.NoError(t, err)
	require.Nil(t, stored)
}

func TestPersonalInstanceStatsReportManagedDirectories(t *testing.T) {
	inst := bootInstance(context.Background(), t, instanceOptions{personal: true})
	inst.createAdmin(t)
	token := inst.signIn(t)

	status, body := inst.connect(t, apiconnect.InstanceServiceGetInstanceStatsProcedure, "", nil)
	require.Equal(t, http.StatusUnauthorized, status, "%s", body)
	require.NotContains(t, string(body), inst.profile.Data)

	status, body = inst.connect(t, apiconnect.InstanceServiceGetInstanceStatsProcedure, token, nil)
	require.Equal(t, http.StatusOK, status, "%s", body)
	var stats apipb.InstanceStats
	require.NoError(t, protojson.Unmarshal(body, &stats))
	require.Equal(t, inst.profile.Data, stats.GetDataDirectory())
	require.Equal(t, filepath.Join(inst.profile.Data, store.MemoBodyDirectory), stats.GetNotesPath())
	require.Equal(t, filepath.Join(inst.profile.Data, store.AttachmentDirectory), stats.GetAttachmentsDirectory())
	require.Equal(t, "sqlite", stats.GetDatabase().GetDriver())
	require.Positive(t, stats.GetDatabase().GetSizeBytes())
	require.Positive(t, stats.GetLocalStorageBytes())
	require.NotContains(t, string(body), inst.profile.DSN)
}
