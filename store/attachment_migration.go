package store

import (
	"bytes"
	"context"
	"path/filepath"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/usememos/memos/internal/random"
	storepb "github.com/usememos/memos/proto/gen/store"
)

// MigrateLocalAttachments moves legacy SQLite blobs into the data directory.
// Call it after schema migration, before serving
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
		attachment.StorageType == storepb.AttachmentStorageType_EXTERNAL ||
		attachment.StorageType == storepb.AttachmentStorageType_S3 {
		return nil
	}
	if attachment.Blob == nil && attachment.Size > 0 {
		return errors.New("attachment database content is missing")
	}
	reference, err := s.publishMigratedAttachment(ctx, attachment.Blob)
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

func (s *Store) publishMigratedAttachment(ctx context.Context, content []byte) (string, error) {
	assets, err := OpenAttachmentDirectory(s.profile.Data)
	if err != nil {
		return "", err
	}
	defer assets.Close()

	tempName, written, err := StageAttachmentFile(ctx, assets, bytes.NewReader(content))
	if err != nil {
		return "", err
	}
	defer assets.Remove(tempName)
	if written != int64(len(content)) {
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
