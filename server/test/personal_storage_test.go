package test

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

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

	_, err := inst.server.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_STORAGE,
		Value: &storepb.InstanceSetting_StorageSetting{StorageSetting: &storepb.InstanceStorageSetting{
			StorageType: storepb.InstanceStorageSetting_S3, FilepathTemplate: "legacy/{filename}",
			S3Config: &storepb.StorageS3Config{
				Endpoint: "https://private-storage.example.test", Bucket: "previous-bucket",
				AccessKeyId: "private-id", AccessKeySecret: "private-secret",
			},
		}},
	})
	require.NoError(t, err)
	before, err := inst.server.Store.GetInstanceStorageSetting(ctx)
	require.NoError(t, err)
	require.Equal(t, storepb.InstanceStorageSetting_S3, before.StorageType)

	const name = "instance/settings/STORAGE"
	const path = "/api/" + name
	status, body := inst.do(t, http.MethodGet, path, "", nil)
	require.Equal(t, http.StatusUnauthorized, status, "%s", body)
	for _, request := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, path, nil},
		{http.MethodPost, apiconnect.InstanceServiceGetInstanceSettingProcedure, map[string]string{"name": name}},
	} {
		status, body = inst.do(t, request.method, request.path, token, request.body)
		require.Equal(t, http.StatusOK, status, "%s", body)
		var got apipb.InstanceSetting
		require.NoError(t, protojson.Unmarshal(body, &got))
		setting := got.GetStorageSetting()
		require.NotNil(t, setting)
		require.Equal(t, apipb.InstanceSetting_StorageSetting_LOCAL, setting.GetStorageType())
		require.Equal(t, "local", setting.GetDefaultStorageId())
		require.Equal(t, store.DefaultAttachmentPathTemplate, setting.GetFilepathTemplate())
		require.Len(t, setting.GetStorages(), 1)
		require.Nil(t, setting.GetS3Config())
		require.NotContains(t, string(body), "private-storage.example.test")
	}
	for _, update := range []map[string]string{
		{"storageType": "DATABASE"},
		{"storageType": "LOCAL", "filepathTemplate": "../alternative/{filename}"},
	} {
		status, body = inst.do(t, http.MethodPatch, path, token, map[string]any{"storageSetting": update})
		require.Equal(t, http.StatusBadRequest, status, "%s", body)
		require.Contains(t, string(body), "attachment storage is managed by Cat_two")
	}
	after, err := inst.server.Store.GetInstanceStorageSetting(ctx)
	require.NoError(t, err)
	require.True(t, proto.Equal(before, after), "the legacy provider registry is kept for migration")
}

func TestPersonalInstanceStatsReportManagedDirectories(t *testing.T) {
	inst := bootInstance(context.Background(), t, instanceOptions{personal: true})
	inst.createAdmin(t)
	token := inst.signIn(t)

	const path = "/api/instance/stats"
	status, body := inst.do(t, http.MethodGet, path, "", nil)
	require.Equal(t, http.StatusUnauthorized, status, "%s", body)
	require.NotContains(t, string(body), inst.profile.Data)

	status, body = inst.do(t, http.MethodGet, path, token, nil)
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
