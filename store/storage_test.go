package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/provider/storage/s3"
	"github.com/usememos/memos/store"
)

func TestNormalizeInstanceStorageSettingMigratesLegacyS3Config(t *testing.T) {
	setting := legacyS3StorageSetting("https://s3.example.com", "memos", "secret")

	store.NormalizeInstanceStorageSetting(setting)

	require.Len(t, setting.Storages, 1)
	require.NotEmpty(t, setting.DefaultStorageId)
	configuredStorage := store.GetDefaultStorage(setting)
	require.Equal(t, storepb.StorageType_STORAGE_TYPE_S3, configuredStorage.Type)
	require.Equal(t, setting.DefaultStorageId, configuredStorage.Id)
	require.Equal(t, "secret", configuredStorage.GetS3Config().AccessKeySecret)
	require.NotSame(t, configuredStorage.GetS3Config(), setting.S3Config)
	require.True(t, proto.Equal(configuredStorage.GetS3Config(), setting.S3Config))
}

func TestNormalizeInstanceStorageSettingKeepsMostRecentlyActivatedStorageFirst(t *testing.T) {
	recent := s3Storage("recent", "recent-bucket")
	old := s3Storage("old", "old-bucket")
	local := &storepb.Storage{Id: "local", Type: storepb.StorageType_STORAGE_TYPE_LOCAL}
	setting := &storepb.InstanceStorageSetting{
		DefaultStorageId: recent.Id,
		Storages:         []*storepb.Storage{old, recent, local},
	}

	store.NormalizeInstanceStorageSetting(setting)
	require.Equal(t, []string{"recent", "old", "local"}, storageIDs(setting.Storages))

	setting.DefaultStorageId = local.Id
	store.NormalizeInstanceStorageSetting(setting)
	require.Equal(t, []string{"local", "recent", "old"}, storageIDs(setting.Storages))
	require.Equal(t, "recent-bucket", setting.S3Config.GetBucket())
}

func TestNormalizeInstanceStorageSettingSelfHealsS3WithoutConfig(t *testing.T) {
	setting := &storepb.InstanceStorageSetting{StorageType: storepb.InstanceStorageSetting_S3}

	store.NormalizeInstanceStorageSetting(setting)

	require.Equal(t, "local", setting.DefaultStorageId)
	require.Equal(t, storepb.StorageType_STORAGE_TYPE_LOCAL, store.GetDefaultStorage(setting).GetType())
	for _, configuredStorage := range setting.Storages {
		require.NotEmpty(t, configuredStorage.Id)
	}
}

func TestNormalizeInstanceStorageSettingUnspecifiedTypeDefaultsToLocal(t *testing.T) {
	// Matches the 0.31 migration and the pre-registry runtime default: a setting
	// that never chose a type stays LOCAL even when an S3 config is present.
	setting := &storepb.InstanceStorageSetting{
		S3Config: &storepb.StorageS3Config{
			AccessKeyId:     "access-key",
			AccessKeySecret: "secret",
			Endpoint:        "https://s3.example.com",
			Region:          "us-east-1",
			Bucket:          "memos",
		},
	}

	store.NormalizeInstanceStorageSetting(setting)

	require.Equal(t, storepb.StorageType_STORAGE_TYPE_LOCAL, store.GetDefaultStorage(setting).GetType())
	var s3Count int
	for _, configuredStorage := range setting.Storages {
		if configuredStorage.GetType() == storepb.StorageType_STORAGE_TYPE_S3 {
			s3Count++
		}
	}
	require.Equal(t, 1, s3Count, "the legacy S3 config must stay registered as a selectable storage")
}

func TestResolveStorageFallsBackWhenStorageIDMissing(t *testing.T) {
	setting := legacyS3StorageSetting("https://s3.example.com", "memos", "secret")
	store.NormalizeInstanceStorageSetting(setting)

	// The registry may be rebuilt without preserving IDs (restored backup,
	// rollback round-trip); a dangling reference must fall back to the namespace
	// chain instead of permanently orphaning the attachment.
	resolved, err := store.ResolveStorage(setting, "s3-dangling", nil)
	require.NoError(t, err)
	require.Equal(t, setting.DefaultStorageId, resolved.Id)

	embedded := &storepb.StorageS3Config{
		AccessKeyId:     "old-access-key",
		AccessKeySecret: "old-secret",
		Endpoint:        "https://s3.example.com/",
		Region:          "us-east-1",
		Bucket:          "memos",
	}
	resolved, err = store.ResolveStorage(setting, "s3-dangling", embedded)
	require.NoError(t, err)
	require.Equal(t, setting.DefaultStorageId, resolved.Id)

	_, err = store.ResolveStorage(&storepb.InstanceStorageSetting{}, "s3-dangling", nil)
	require.ErrorContains(t, err, `storage "s3-dangling" is not configured`)
}

func TestResolveStorageDriverByStorageID(t *testing.T) {
	stores := &store.Store{}
	setting := &storepb.InstanceStorageSetting{
		DefaultStorageId: "primary",
		Storages: []*storepb.Storage{
			{
				Id:   "primary",
				Type: storepb.StorageType_STORAGE_TYPE_S3,
				Config: &storepb.Storage_S3Config{S3Config: &storepb.StorageS3Config{
					AccessKeyId:     "access-key",
					AccessKeySecret: "secret",
					Endpoint:        "https://s3.example.com",
					Region:          "us-east-1",
					Bucket:          "memos",
				}},
			},
		},
	}

	driver, err := stores.ResolveStorageDriver(context.Background(), setting, "primary", nil)
	require.NoError(t, err)
	require.IsType(t, &s3.Driver{}, driver)
	repeatedDriver, err := stores.ResolveStorageDriver(context.Background(), setting, "primary", nil)
	require.NoError(t, err)
	require.Same(t, driver, repeatedDriver)

	_, err = stores.ResolveStorageDriver(context.Background(), setting, "missing", nil)
	require.ErrorContains(t, err, `storage "missing" is not configured`)
}

func TestResolveStorageDriverSupportsLegacyEmbeddedConfig(t *testing.T) {
	stores := &store.Store{}
	legacyConfig := &storepb.StorageS3Config{
		AccessKeyId:     "access-key",
		AccessKeySecret: "secret",
		Endpoint:        "https://legacy-s3.example.com",
		Region:          "us-east-1",
		Bucket:          "legacy",
	}

	driver, err := stores.ResolveStorageDriver(context.Background(), nil, "", legacyConfig)
	require.NoError(t, err)
	require.IsType(t, &s3.Driver{}, driver)
}

func TestStorageDriverCacheIncludesResolvedConfiguration(t *testing.T) {
	ctx := context.Background()
	stores := &store.Store{}

	retainedOldStorage := s3Storage("stable", "memos")
	retainedOldStorage.GetS3Config().AccessKeyId = "old-access-key"
	retainedOldStorage.GetS3Config().AccessKeySecret = "old-secret"
	currentStorage := proto.CloneOf(retainedOldStorage)
	currentStorage.GetS3Config().AccessKeyId = "new-access-key"
	currentStorage.GetS3Config().AccessKeySecret = "new-secret"

	// Model an in-flight request repopulating an empty cache after credential
	// rotation, then a current request resolving the same stable storage ID.
	staleDriver, err := stores.StorageDriver(ctx, retainedOldStorage)
	require.NoError(t, err)
	currentDriver, err := stores.StorageDriver(ctx, currentStorage)
	require.NoError(t, err)
	require.NotSame(t, staleDriver, currentDriver)

	staleS3Driver, ok := staleDriver.(*s3.Driver)
	require.True(t, ok)
	staleCredentials, err := staleS3Driver.Client.Options().Credentials.Retrieve(ctx)
	require.NoError(t, err)
	require.Equal(t, "old-access-key", staleCredentials.AccessKeyID)

	currentS3Driver, ok := currentDriver.(*s3.Driver)
	require.True(t, ok)
	currentCredentials, err := currentS3Driver.Client.Options().Credentials.Retrieve(ctx)
	require.NoError(t, err)
	require.Equal(t, "new-access-key", currentCredentials.AccessKeyID)

	repeatedDriver, err := stores.StorageDriver(ctx, currentStorage)
	require.NoError(t, err)
	require.Same(t, currentDriver, repeatedDriver)
}

func TestResolveStorageDriverUsesCurrentCredentialsForLegacyAttachment(t *testing.T) {
	stores := &store.Store{}
	legacyConfig := &storepb.StorageS3Config{
		AccessKeyId:     "old-access-key",
		AccessKeySecret: "old-secret",
		Endpoint:        "https://s3.example.com/",
		Region:          "us-east-1",
		Bucket:          "memos",
	}
	setting := &storepb.InstanceStorageSetting{
		DefaultStorageId: "primary",
		Storages: []*storepb.Storage{
			{
				Id:   "primary",
				Type: storepb.StorageType_STORAGE_TYPE_S3,
				Config: &storepb.Storage_S3Config{S3Config: &storepb.StorageS3Config{
					AccessKeyId:     "new-access-key",
					AccessKeySecret: "new-secret",
					Endpoint:        "https://s3.example.com",
					Region:          "us-east-1",
					Bucket:          "memos",
					UsePathStyle:    true,
				}},
			},
		},
	}

	driver, err := stores.ResolveStorageDriver(context.Background(), setting, "", legacyConfig)
	require.NoError(t, err)
	s3Driver, ok := driver.(*s3.Driver)
	require.True(t, ok)
	require.True(t, s3Driver.Client.Options().UsePathStyle)
}

func legacyS3StorageSetting(endpoint, bucket, secret string) *storepb.InstanceStorageSetting {
	return &storepb.InstanceStorageSetting{
		StorageType: storepb.InstanceStorageSetting_S3,
		S3Config: &storepb.StorageS3Config{
			AccessKeyId:     "access-key",
			AccessKeySecret: secret,
			Endpoint:        endpoint,
			Region:          "us-east-1",
			Bucket:          bucket,
		},
	}
}

func s3Storage(id, bucket string) *storepb.Storage {
	return &storepb.Storage{
		Id:   id,
		Type: storepb.StorageType_STORAGE_TYPE_S3,
		Config: &storepb.Storage_S3Config{S3Config: &storepb.StorageS3Config{
			Endpoint: "https://s3.example.com",
			Region:   "us-east-1",
			Bucket:   bucket,
		}},
	}
}

func storageIDs(storages []*storepb.Storage) []string {
	ids := make([]string, 0, len(storages))
	for _, configuredStorage := range storages {
		ids = append(ids, configuredStorage.Id)
	}
	return ids
}
