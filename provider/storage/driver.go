package storage

import (
	"context"

	"github.com/pkg/errors"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/provider/storage/s3"
)

// ObjectStream is the content and length of a legacy attachment object.
type ObjectStream = s3.ObjectStream

// Driver reads complete objects for the attachment storage migration.
type Driver interface {
	GetObjectStream(ctx context.Context, key string) (*ObjectStream, error)
}

// NewDriver creates a read-only driver for a legacy object store.
func NewDriver(ctx context.Context, configuredStorage *storepb.Storage) (Driver, error) {
	if configuredStorage == nil {
		return nil, errors.New("storage is required")
	}
	switch configuredStorage.Type {
	case storepb.StorageType_STORAGE_TYPE_S3:
		if configuredStorage.GetS3Config() == nil {
			return nil, errors.Errorf("S3 config is missing for storage %q", configuredStorage.Id)
		}
		return s3.NewDriver(ctx, configuredStorage.GetS3Config())
	default:
		return nil, errors.Errorf("storage %q has unsupported driver type %s", configuredStorage.Id, configuredStorage.Type.String())
	}
}
