package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/provider/storage"
)

const (
	databaseStorageID = "database"
	localStorageID    = "local"
)

type storageDriverCacheKey [sha256.Size]byte

// NormalizeInstanceStorageSetting populates the named storage model from legacy
// fields and keeps the legacy fields synchronized for older clients.
func NormalizeInstanceStorageSetting(setting *storepb.InstanceStorageSetting) {
	if setting == nil {
		return
	}

	normalized := make([]*storepb.Storage, 0, len(setting.Storages)+1)
	seenIDs := map[string]bool{}
	for _, storage := range setting.Storages {
		if storage == nil {
			continue
		}
		normalizeStorage(storage)
		if storage.Id == "" || seenIDs[storage.Id] {
			continue
		}
		seenIDs[storage.Id] = true
		normalized = append(normalized, storage)
	}
	setting.Storages = normalized
	if setting.S3Config != nil {
		legacyS3Storage := &storepb.Storage{
			Type:   storepb.StorageType_STORAGE_TYPE_S3,
			Config: &storepb.Storage_S3Config{S3Config: setting.S3Config},
		}
		// Equivalence is checked before normalizeStorage so the ID hash is only
		// computed when the legacy config actually needs to be registered.
		if findEquivalentStorage(setting.Storages, legacyS3Storage) == nil {
			normalizeStorage(legacyS3Storage)
			if legacyS3Storage.Id != "" {
				setting.Storages = append(setting.Storages, legacyS3Storage)
			}
		}
	}

	if setting.DefaultStorageId == "" {
		legacyStorage := storageFromLegacySetting(setting)
		if legacyStorage != nil {
			if existing := findEquivalentStorage(setting.Storages, legacyStorage); existing != nil {
				setting.DefaultStorageId = existing.Id
			} else if legacyStorage.Id != "" {
				// An ID-less storage (S3 declared without a config) would wedge the
				// fallback below into selecting it forever; let the LOCAL fallback
				// self-heal instead.
				setting.Storages = append(setting.Storages, legacyStorage)
				setting.DefaultStorageId = legacyStorage.Id
			}
		}
	}

	if FindStorage(setting, setting.DefaultStorageId) == nil {
		fallback := builtinStorage(storepb.StorageType_STORAGE_TYPE_LOCAL)
		if len(setting.Storages) > 0 {
			fallback = setting.Storages[0]
		} else {
			setting.Storages = append(setting.Storages, fallback)
		}
		setting.DefaultStorageId = fallback.Id
	}
	moveDefaultStorageFirst(setting)

	synchronizeLegacyStorageFields(setting)
}

// ResolveStorage resolves the configured storage referenced by an attachment.
// It returns a registry entry when one matches — its Id is the stable identity —
// or a synthesized storage wrapping the legacy config otherwise (empty Id).
// legacyS3Config supports attachments written before storage IDs were introduced.
func ResolveStorage(
	setting *storepb.InstanceStorageSetting,
	storageID string,
	legacyS3Config *storepb.StorageS3Config,
) (*storepb.Storage, error) {
	if storageID != "" {
		if configuredStorage := FindStorage(setting, storageID); configuredStorage != nil {
			return configuredStorage, nil
		}
		// The registry may have been rebuilt without preserving IDs (restored
		// backup, rollback round-trip); fall through to the legacy chain rather
		// than permanently orphaning the attachment.
	}

	s3Config := legacyS3Config
	if s3Config == nil {
		s3Config = setting.GetS3Config()
	}
	if s3Config != nil {
		// Prefer the configured storage addressing the same namespace so legacy
		// attachments pick up rotated credentials and transport options.
		legacyStorage := &storepb.Storage{
			Type:   storepb.StorageType_STORAGE_TYPE_S3,
			Config: &storepb.Storage_S3Config{S3Config: s3Config},
		}
		if configuredStorage := findEquivalentStorage(setting.GetStorages(), legacyStorage); configuredStorage != nil {
			return configuredStorage, nil
		}
		return legacyStorage, nil
	}

	if storageID != "" {
		return nil, errors.Errorf("storage %q is not configured", storageID)
	}
	return nil, errors.New("attachment storage is not configured")
}

// ResolveStorageDriver resolves the configured storage referenced by an attachment
// and returns its driver, reusing the Store cache for named storage. legacyS3Config
// supports attachments written before storage IDs were introduced.
func (s *Store) ResolveStorageDriver(
	ctx context.Context,
	setting *storepb.InstanceStorageSetting,
	storageID string,
	legacyS3Config *storepb.StorageS3Config,
) (storage.Driver, error) {
	resolvedStorage, err := ResolveStorage(setting, storageID, legacyS3Config)
	if err != nil {
		return nil, err
	}
	return s.StorageDriver(ctx, resolvedStorage)
}

// StorageDriver returns the driver for a resolved storage, reusing cached
// clients so request paths do not rebuild an S3 client (config load, HTTP
// transport) per call. ID-less legacy storages are not cached.
func (s *Store) StorageDriver(ctx context.Context, resolvedStorage *storepb.Storage) (storage.Driver, error) {
	if resolvedStorage.GetId() == "" {
		return storage.NewDriver(ctx, resolvedStorage)
	}
	cacheKey, err := newStorageDriverCacheKey(resolvedStorage)
	if err != nil {
		return nil, err
	}

	s.storageDriverMu.Lock()
	cacheGeneration := s.storageDriverGeneration
	if driver, ok := s.storageDriverCache[cacheKey]; ok {
		s.storageDriverMu.Unlock()
		return driver, nil
	}
	s.storageDriverMu.Unlock()

	driver, err := storage.NewDriver(ctx, resolvedStorage)
	if err != nil {
		return nil, err
	}

	s.storageDriverMu.Lock()
	defer s.storageDriverMu.Unlock()
	if cachedDriver, ok := s.storageDriverCache[cacheKey]; ok {
		return cachedDriver, nil
	}
	// A setting change while the driver was being constructed invalidates this
	// cache miss. The caller can still use the resolved driver for this request,
	// but a later request must resolve and cache the current configuration.
	if cacheGeneration != s.storageDriverGeneration {
		return driver, nil
	}
	if s.storageDriverCache == nil {
		s.storageDriverCache = map[storageDriverCacheKey]storage.Driver{}
	}
	s.storageDriverCache[cacheKey] = driver
	return driver, nil
}

func newStorageDriverCacheKey(resolvedStorage *storepb.Storage) (storageDriverCacheKey, error) {
	marshalOptions := proto.MarshalOptions{Deterministic: true}
	data, err := marshalOptions.Marshal(resolvedStorage)
	if err != nil {
		return storageDriverCacheKey{}, errors.Wrap(err, "failed to fingerprint storage configuration")
	}
	return storageDriverCacheKey(sha256.Sum256(data)), nil
}

// resetStorageDriverCache drops cached storage drivers; call whenever the
// STORAGE setting may have changed (credentials or transport options can
// rotate under an unchanged storage ID).
func (s *Store) resetStorageDriverCache() {
	s.storageDriverMu.Lock()
	defer s.storageDriverMu.Unlock()
	s.storageDriverGeneration++
	s.storageDriverCache = nil
}

// FindStorage returns the configured storage with the given stable identifier.
func FindStorage(setting *storepb.InstanceStorageSetting, storageID string) *storepb.Storage {
	if setting == nil || storageID == "" {
		return nil
	}
	for _, storage := range setting.Storages {
		if storage != nil && storage.Id == storageID {
			return storage
		}
	}
	return nil
}

// GetDefaultStorage returns the storage used for new attachments.
func GetDefaultStorage(setting *storepb.InstanceStorageSetting) *storepb.Storage {
	if setting == nil {
		return nil
	}
	return FindStorage(setting, setting.DefaultStorageId)
}

// StorageNamespaceEqual reports whether two configurations address the same
// physical storage namespace. Credentials and transport options are deliberately
// excluded so they can be rotated without changing storage identity.
func StorageNamespaceEqual(a, b *storepb.Storage) bool {
	if a == nil || b == nil || a.Type != b.Type {
		return false
	}
	if a.Type != storepb.StorageType_STORAGE_TYPE_S3 {
		return true
	}
	aConfig, bConfig := a.GetS3Config(), b.GetS3Config()
	if aConfig == nil || bConfig == nil {
		return aConfig == nil && bConfig == nil
	}
	return normalizeEndpoint(aConfig.Endpoint) == normalizeEndpoint(bConfig.Endpoint) &&
		strings.TrimSpace(aConfig.Region) == strings.TrimSpace(bConfig.Region) &&
		strings.TrimSpace(aConfig.Bucket) == strings.TrimSpace(bConfig.Bucket)
}

func storageFromLegacySetting(setting *storepb.InstanceStorageSetting) *storepb.Storage {
	storageType := storepb.StorageType(setting.StorageType)
	if storageType == storepb.StorageType_STORAGE_TYPE_UNSPECIFIED {
		// Match the 0.31 migration and the pre-registry runtime default: an
		// unspecified type is LOCAL even when a legacy S3 config is present
		// (the config is still registered as a selectable storage above).
		storageType = storepb.StorageType_STORAGE_TYPE_LOCAL
	}
	storage := builtinStorage(storageType)
	if storageType == storepb.StorageType_STORAGE_TYPE_S3 {
		storage.Config = &storepb.Storage_S3Config{S3Config: setting.S3Config}
		normalizeStorage(storage)
	}
	return storage
}

func builtinStorage(storageType storepb.StorageType) *storepb.Storage {
	storage := &storepb.Storage{Type: storageType}
	normalizeStorage(storage)
	return storage
}

func normalizeStorage(storage *storepb.Storage) {
	if storage == nil {
		return
	}
	if storage.Type == storepb.StorageType_STORAGE_TYPE_UNSPECIFIED && storage.GetS3Config() != nil {
		storage.Type = storepb.StorageType_STORAGE_TYPE_S3
	}
	if storage.Id == "" {
		storage.Id = storageID(storage)
	}
	if strings.TrimSpace(storage.Name) == "" {
		storage.Name = defaultStorageName(storage)
	}
}

func storageID(storage *storepb.Storage) string {
	if storage == nil {
		return ""
	}
	switch storage.Type {
	case storepb.StorageType_STORAGE_TYPE_DATABASE:
		return databaseStorageID
	case storepb.StorageType_STORAGE_TYPE_LOCAL:
		return localStorageID
	case storepb.StorageType_STORAGE_TYPE_S3:
		config := storage.GetS3Config()
		if config == nil {
			return ""
		}
		namespace := strings.Join([]string{
			normalizeEndpoint(config.Endpoint),
			strings.TrimSpace(config.Region),
			strings.TrimSpace(config.Bucket),
		}, "\x00")
		hash := sha256.Sum256([]byte(namespace))
		return "s3-" + hex.EncodeToString(hash[:])[:16]
	default:
		return ""
	}
}

func defaultStorageName(storage *storepb.Storage) string {
	if storage == nil {
		return ""
	}
	switch storage.Type {
	case storepb.StorageType_STORAGE_TYPE_DATABASE:
		return "Database"
	case storepb.StorageType_STORAGE_TYPE_LOCAL:
		return "Local"
	case storepb.StorageType_STORAGE_TYPE_S3:
		if bucket := strings.TrimSpace(storage.GetS3Config().GetBucket()); bucket != "" {
			return bucket
		}
		return "S3"
	default:
		return "Storage"
	}
}

func findEquivalentStorage(storages []*storepb.Storage, target *storepb.Storage) *storepb.Storage {
	for _, storage := range storages {
		if StorageNamespaceEqual(storage, target) {
			return storage
		}
	}
	return nil
}

// moveDefaultStorageFirst keeps the registry ordered by most recent activation.
// This lets clients fall back to the most recently used storage of a given type
// while attachments continue to resolve storages by ID.
func moveDefaultStorageFirst(setting *storepb.InstanceStorageSetting) {
	for i, storage := range setting.Storages {
		if storage.Id != setting.DefaultStorageId || i == 0 {
			continue
		}
		copy(setting.Storages[1:i+1], setting.Storages[:i])
		setting.Storages[0] = storage
		return
	}
}

func synchronizeLegacyStorageFields(setting *storepb.InstanceStorageSetting) {
	storage := GetDefaultStorage(setting)
	if storage == nil {
		return
	}
	setting.StorageType = storepb.InstanceStorageSetting_StorageType(storage.Type)
	if storage.GetS3Config() != nil {
		setting.S3Config = proto.CloneOf(storage.GetS3Config())
		return
	}
	// Keep the most recently configured S3 storage available to legacy
	// attachments that predate both storage IDs and embedded configurations.
	for _, configuredStorage := range setting.Storages {
		if configuredStorage.GetS3Config() != nil {
			setting.S3Config = proto.CloneOf(configuredStorage.GetS3Config())
			return
		}
	}
	setting.S3Config = nil
}

func normalizeEndpoint(endpoint string) string {
	return strings.TrimRight(strings.TrimSpace(endpoint), "/")
}
