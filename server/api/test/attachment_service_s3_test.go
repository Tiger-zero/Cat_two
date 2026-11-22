package test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/store"
)

func TestAttachmentWritesIgnoreLegacyStorageConfiguration(t *testing.T) {
	ctx := t.Context()
	ts := NewTestService(t)
	defer ts.Cleanup()
	user, err := ts.CreateRegularUser(ctx, "local-attachment-owner")
	require.NoError(t, err)
	userCtx := ts.CreateUserContext(ctx, user.ID)

	for _, storageType := range []storepb.InstanceStorageSetting_StorageType{
		storepb.InstanceStorageSetting_DATABASE,
		storepb.InstanceStorageSetting_S3,
		storepb.InstanceStorageSetting_LOCAL,
	} {
		t.Run(storageType.String(), func(t *testing.T) {
			legacy := &storepb.InstanceStorageSetting{
				StorageType:      storageType,
				FilepathTemplate: filepath.Join(t.TempDir(), "{filename}"),
			}
			if storageType == storepb.InstanceStorageSetting_S3 {
				legacy.S3Config = &storepb.StorageS3Config{
					AccessKeyId: "unused", AccessKeySecret: "unused", Endpoint: "https://example.invalid", Region: "test", Bucket: "legacy",
				}
			}
			_, err := ts.Store.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
				Key:   storepb.InstanceSettingKey_STORAGE,
				Value: &storepb.InstanceSetting_StorageSetting{StorageSetting: legacy},
			})
			require.NoError(t, err)

			const name, body = "note.txt", "stored under assets"
			attachment, err := ts.Service.CreateAttachment(userCtx, &apipb.CreateAttachmentRequest{Attachment: &apipb.Attachment{
				Filename: name, Type: "text/plain", Content: []byte(body),
			}})
			require.NoError(t, err)
			uid, err := api.ExtractAttachmentUIDFromName(attachment.Name)
			require.NoError(t, err)
			row, err := ts.Store.GetAttachment(ctx, &store.FindAttachment{UID: &uid, GetBlob: true})
			require.NoError(t, err)
			require.Equal(t, storepb.AttachmentStorageType_LOCAL, row.StorageType)
			require.Nil(t, row.Blob)
			require.Nil(t, row.Payload.GetS3Object())
			require.Regexp(t, `^assets/[0-9]+_[0-9a-f-]+_note\.txt$`, row.Reference)
			require.NoFileExists(t, filepath.Join(filepath.Dir(legacy.FilepathTemplate), name))
			data, err := ts.Service.GetAttachmentBlob(ctx, row)
			require.NoError(t, err)
			require.Equal(t, []byte(body), data)
		})
	}
}
