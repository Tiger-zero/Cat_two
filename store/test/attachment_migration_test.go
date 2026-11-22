package test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/testutil/fakes3"
	"github.com/usememos/memos/internal/version"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
	"github.com/usememos/memos/store/db"
)

func TestMigrateLocalAttachments(t *testing.T) {
	ctx := t.Context()
	testStore := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, testStore.Close()) })
	owner, err := testStore.CreateUser(ctx, &store.User{Username: "attachment-migration", Role: store.RoleAdmin, PasswordHash: "hash"})
	require.NoError(t, err)
	memo, err := testStore.CreateMemo(ctx, &store.Memo{UID: "attachment-migration-memo", CreatorID: owner.ID, Visibility: store.Private})
	require.NoError(t, err)

	const objectKey = "legacy/reused.txt"
	fakeS3 := fakes3.New(t, "first-bucket", "second-bucket")
	require.NoError(t, fakeS3.PutObject("first-bucket", objectKey, "text/plain", []byte("registered object")))
	require.NoError(t, fakeS3.PutObject("second-bucket", objectKey, "text/plain", []byte("embedded object")))
	_, err = testStore.UpsertInstanceSetting(ctx, &storepb.InstanceSetting{
		Key: storepb.InstanceSettingKey_STORAGE,
		Value: &storepb.InstanceSetting_StorageSetting{StorageSetting: &storepb.InstanceStorageSetting{
			DefaultStorageId: "registered",
			Storages: []*storepb.Storage{{
				Id: "registered", Type: storepb.StorageType_STORAGE_TYPE_S3,
				Config: &storepb.Storage_S3Config{S3Config: fakeS3.Config("first-bucket")},
			}},
		}},
	})
	require.NoError(t, err)

	absolute := filepath.Join(t.TempDir(), "old-absolute.txt")
	require.NoError(t, os.WriteFile(absolute, []byte("already local"), 0o600))
	create := func(create *store.Attachment) *store.Attachment {
		t.Helper()
		create.CreatorID = owner.ID
		create.Filename = "report.txt"
		create.Type = "text/plain"
		attachment, err := testStore.CreateAttachment(ctx, create)
		require.NoError(t, err)
		return attachment
	}
	dbContent := []byte{1, 2, 0, 255, 17}
	database := create(&store.Attachment{UID: "database-attachment", Blob: dbContent, Size: int64(len(dbContent))})
	databaseEmpty := create(&store.Attachment{UID: "database-empty-attachment", Blob: []byte{}})
	meta := &storepb.MotionMedia{GroupId: "paired-media"}
	registered := create(&store.Attachment{UID: "registered-object", MemoID: &memo.ID, Size: 17,
		StorageType: storepb.AttachmentStorageType_S3, Payload: &storepb.AttachmentPayload{
			MotionMedia: meta,
			Payload: &storepb.AttachmentPayload_S3Object_{S3Object: &storepb.AttachmentPayload_S3Object{
				Key: objectKey, StorageId: "registered",
			}},
		},
	})
	embedded := create(&store.Attachment{UID: "embedded-object", Size: 15,
		StorageType: storepb.AttachmentStorageType_S3, Payload: &storepb.AttachmentPayload{
			Payload: &storepb.AttachmentPayload_S3Object_{S3Object: &storepb.AttachmentPayload_S3Object{
				Key: objectKey, S3Config: fakeS3.Config("second-bucket"),
			}},
		},
	})
	local := create(&store.Attachment{UID: "already-local", StorageType: storepb.AttachmentStorageType_LOCAL, Reference: absolute})
	external := create(&store.Attachment{UID: "external-attachment", StorageType: storepb.AttachmentStorageType_EXTERNAL, Reference: "https://example.test/asset"})
	_, err = testStore.ApplyMemoTrash(ctx, &store.MemoTrashMutation{
		MemoID: memo.ID, ActorUserID: owner.ID, Action: store.MemoTrashMove, NowTs: time.Now().Unix(),
	})
	require.NoError(t, err)

	assets := filepath.Join(testStore.GetDataDir(), store.AttachmentDirectory)
	require.NoError(t, testStore.MigrateLocalAttachments(ctx))
	for _, pair := range []struct {
		original *store.Attachment
		content  []byte
	}{
		{database, dbContent},
		{databaseEmpty, []byte{}},
		{registered, []byte("registered object")},
		{embedded, []byte("embedded object")},
	} {
		row, err := testStore.GetAttachment(ctx, &store.FindAttachment{ID: &pair.original.ID, GetBlob: true})
		require.NoError(t, err)
		require.Equal(t, pair.original.ID, row.ID)
		require.Equal(t, pair.original.UID, row.UID)
		require.Equal(t, pair.original.MemoID, row.MemoID)
		require.Equal(t, pair.original.Filename, row.Filename)
		require.Equal(t, pair.original.UpdatedTs, row.UpdatedTs)
		require.Equal(t, pair.original.Size, row.Size)
		require.Nil(t, row.Blob)
		require.Nil(t, row.Payload.GetS3Object())
		require.Equal(t, pair.original.Payload.GetMotionMedia(), row.Payload.GetMotionMedia())
		require.Equal(t, storepb.AttachmentStorageType_LOCAL, row.StorageType)
		require.Equal(t, store.AttachmentDirectory, filepath.Dir(row.Reference))
		path := filepath.Join(testStore.GetDataDir(), row.Reference)
		bytes, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, pair.content, bytes)
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Zero(t, info.Mode().Perm()&0o077)
	}
	for _, original := range []*store.Attachment{local, external} {
		row, err := testStore.GetAttachment(ctx, &store.FindAttachment{ID: &original.ID})
		require.NoError(t, err)
		require.Equal(t, original.StorageType, row.StorageType)
		require.Equal(t, original.Reference, row.Reference)
	}
	require.NoError(t, testStore.MigrateLocalAttachments(ctx))
	entries, err := os.ReadDir(assets)
	require.NoError(t, err)
	require.Len(t, entries, 4)
	for i, bucket := range []string{"first-bucket", "second-bucket"} {
		bytes, err := fakeS3.GetObject(bucket, objectKey)
		require.NoError(t, err)
		require.Equal(t, []string{"registered object", "embedded object"}[i], string(bytes))
	}
}

func TestMigrateLocalAttachmentsRetryAfterFailure(t *testing.T) {
	ctx := t.Context()
	testStore := NewTestingStore(ctx, t)
	t.Cleanup(func() { require.NoError(t, testStore.Close()) })
	owner, err := testStore.CreateUser(ctx, &store.User{Username: "attachment-migration-retry", Role: store.RoleAdmin, PasswordHash: "hash"})
	require.NoError(t, err)
	legacy, err := testStore.CreateAttachment(ctx, &store.Attachment{
		UID: "legacy-retry", CreatorID: owner.ID, Filename: "legacy.txt", Size: 6, Blob: []byte("legacy"),
	})
	require.NoError(t, err)
	stale := *legacy
	stale.UpdatedTs++
	updated, err := testStore.GetDriver().LocalizeAttachment(ctx, &stale, "assets/obsolete", "{}")
	require.NoError(t, err)
	require.False(t, updated)
	assets := filepath.Join(testStore.GetDataDir(), store.AttachmentDirectory)
	outside := t.TempDir()
	require.NoError(t, os.Symlink(outside, assets))
	require.ErrorContains(t, testStore.MigrateLocalAttachments(ctx), "attachment directory must be a directory")
	entries, err := os.ReadDir(outside)
	require.NoError(t, err)
	require.Empty(t, entries)
	unmigrated, err := testStore.GetAttachment(ctx, &store.FindAttachment{ID: &legacy.ID, GetBlob: true})
	require.NoError(t, err)
	require.Equal(t, legacy.Blob, unmigrated.Blob)
	require.Equal(t, legacy.StorageType, unmigrated.StorageType)
	require.NoError(t, os.Remove(assets))

	fakeS3 := fakes3.New(t, "migration-retry")
	missing, err := testStore.CreateAttachment(ctx, &store.Attachment{
		UID: "legacy-s3-retry", CreatorID: owner.ID, Filename: "legacy.txt", Size: 6,
		StorageType: storepb.AttachmentStorageType_S3,
		Payload: &storepb.AttachmentPayload{Payload: &storepb.AttachmentPayload_S3Object_{
			S3Object: &storepb.AttachmentPayload_S3Object{Key: "temporary-missing", S3Config: fakeS3.Config("migration-retry")},
		}},
	})
	require.NoError(t, err)
	require.ErrorContains(t, testStore.MigrateLocalAttachments(ctx), "failed to retrieve legacy object from storage")
	migrated, err := testStore.GetAttachment(ctx, &store.FindAttachment{ID: &legacy.ID})
	require.NoError(t, err)
	require.Equal(t, storepb.AttachmentStorageType_LOCAL, migrated.StorageType)
	waiting, err := testStore.GetAttachment(ctx, &store.FindAttachment{ID: &missing.ID})
	require.NoError(t, err)
	require.Equal(t, storepb.AttachmentStorageType_S3, waiting.StorageType)
	require.NoError(t, fakeS3.PutObject("migration-retry", "temporary-missing", "text/plain", []byte("legacy")))
	require.NoError(t, testStore.MigrateLocalAttachments(ctx))
	retried, err := testStore.GetAttachment(ctx, &store.FindAttachment{ID: &missing.ID})
	require.NoError(t, err)
	require.Equal(t, storepb.AttachmentStorageType_LOCAL, retried.StorageType)
	require.NotEqual(t, migrated.Reference, retried.Reference)
}

func TestMigrateLocalAttachmentsSharedDataDirectory(t *testing.T) {
	ctx := t.Context()
	data := t.TempDir()
	createStore := func(filename string) (*store.Store, *store.Attachment) {
		t.Helper()
		p := &profile.Profile{Data: data, DSN: filepath.Join(data, filename), Driver: "sqlite", Version: version.GetCurrentVersion()}
		driver, err := db.NewDBDriver(p)
		require.NoError(t, err)
		s := store.New(driver, p)
		t.Cleanup(func() { require.NoError(t, s.Close()) })
		require.NoError(t, s.Migrate(ctx))
		owner, err := s.CreateUser(ctx, &store.User{Username: "owner", Role: store.RoleAdmin, PasswordHash: "hash"})
		require.NoError(t, err)
		attachment, err := s.CreateAttachment(ctx, &store.Attachment{
			UID: "same-attachment", CreatorID: owner.ID, Filename: "document.txt", Blob: []byte("same bytes"), Size: 10,
		})
		require.NoError(t, err)
		require.NoError(t, s.MigrateLocalAttachments(ctx))
		migrated, err := s.GetAttachment(ctx, &store.FindAttachment{ID: &attachment.ID})
		require.NoError(t, err)
		return s, migrated
	}
	firstStore, first := createStore("first.db")
	_, second := createStore("second.db")
	require.Equal(t, first.ID, second.ID)
	require.NotEqual(t, first.Reference, second.Reference)
	require.NoError(t, firstStore.DeleteAttachment(ctx, &store.DeleteAttachment{ID: first.ID}))
	require.NoFileExists(t, filepath.Join(data, first.Reference))
	require.FileExists(t, filepath.Join(data, second.Reference))
}
