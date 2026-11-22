package store

import (
	"bytes"
	"context"
	"io"
	"path/filepath"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/usememos/memos/internal/random"
	storepb "github.com/usememos/memos/proto/gen/store"
)

// MigrateLocalAttachments moves legacy database and object storage attachments
// into the data directory. Call it after schema migration, before serving
// requests and while no previous version is writing to the same database.
func (s *Store) MigrateLocalAttachments(ctx context.Context) error {
	const batchSize = 64
	var afterID int32
	for {
		ids, err := s.listLegacyAttachmentIDs(ctx, afterID, batchSize)
		if err != nil {
			return errors.Wrap(err, "failed to list legacy attachments")
		}
		if len(ids) == 0 {
			return nil
		}
		for _, id := range ids {
			if err := s.migrateLocalAttachment(ctx, id); err != nil {
				return errors.Wrapf(err, "failed to migrate attachment %d", id)
			}
		}
		afterID = ids[len(ids)-1]
	}
}

func (s *Store) listLegacyAttachmentIDs(ctx context.Context, afterID int32, limit int) ([]int32, error) {
	rows, err := s.driver.ListLegacyAttachmentIDs(ctx, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int32, 0, limit)
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) migrateLocalAttachment(ctx context.Context, id int32) error {
	attachment, err := s.GetAttachment(ctx, &FindAttachment{ID: &id, GetBlob: true})
	if err != nil {
		return errors.Wrap(err, "failed to load legacy attachment")
	}
	if attachment == nil || attachment.StorageType == storepb.AttachmentStorageType_LOCAL ||
		attachment.StorageType == storepb.AttachmentStorageType_EXTERNAL {
		return nil
	}
	content, length, err := s.openLegacyAttachment(ctx, attachment)
	if err != nil {
		return err
	}
	defer content.Close()
	reference, err := s.publishMigratedAttachment(ctx, content, length)
	if err != nil {
		return err
	}

	metadata := &storepb.AttachmentPayload{}
	if attachment.Payload != nil {
		metadata = proto.CloneOf(attachment.Payload)
		metadata.Payload = nil
	}
	payload, err := protojson.Marshal(metadata)
	if err != nil {
		return errors.Wrap(err, "failed to encode attachment metadata")
	}
	updated, err := s.driver.LocalizeAttachment(ctx, attachment, reference, string(payload))
	if err != nil {
		// An error can arrive after the database has committed. Preserve
		// the file because the row may already point to it.
		return errors.Wrap(err, "failed to update attachment storage")
	}
	if !updated {
		return errors.New("attachment storage changed during migration")
	}
	return nil
}

func (s *Store) openLegacyAttachment(ctx context.Context, attachment *Attachment) (io.ReadCloser, int64, error) {
	if attachment.StorageType != storepb.AttachmentStorageType_S3 {
		if attachment.Blob == nil && attachment.Size > 0 {
			return nil, 0, errors.New("attachment database content is missing")
		}
		return io.NopCloser(bytes.NewReader(attachment.Blob)), int64(len(attachment.Blob)), nil
	}
	object := attachment.Payload.GetS3Object()
	if object == nil || object.Key == "" {
		return nil, 0, errors.New("legacy object storage key is missing")
	}
	setting, err := s.GetInstanceStorageSetting(ctx)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to load legacy storage settings")
	}
	driver, err := s.ResolveStorageDriver(ctx, setting, object.StorageId, object.S3Config)
	if err != nil {
		return nil, 0, errors.New("failed to resolve legacy object storage")
	}
	stream, err := driver.GetObjectStream(ctx, object.Key)
	if err != nil {
		return nil, 0, errors.New("failed to retrieve legacy object from storage")
	}
	return stream.Body, stream.ContentLength, nil
}

func (s *Store) publishMigratedAttachment(ctx context.Context, source io.Reader, length int64) (string, error) {
	assets, err := OpenAttachmentDirectory(s.profile.Data)
	if err != nil {
		return "", err
	}
	defer assets.Close()

	tempName, written, err := StageAttachmentFile(ctx, assets, source)
	if err != nil {
		return "", err
	}
	defer assets.Remove(tempName)
	if length >= 0 && written != length {
		return "", errors.New("legacy attachment content has an unexpected length")
	}

	name := "migrated-" + random.UUID()
	if err := assets.Link(tempName, name); err != nil {
		return "", errors.Wrap(err, "failed to publish migrated attachment")
	}
	if err := SyncAttachmentDirectory(assets); err != nil {
		return "", errors.Wrap(err, "failed to sync the attachment directory")
	}
	return filepath.ToSlash(filepath.Join(AttachmentDirectory, name)), nil
}
