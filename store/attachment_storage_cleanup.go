package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	storepb "github.com/usememos/memos/proto/gen/store"
)

// EnqueueMemoTrashAttachmentCleanup saves deleted attachment metadata within
// the memo purge transaction. Blobs are already removed by that transaction.
func EnqueueMemoTrashAttachmentCleanup(ctx context.Context, tx *sql.Tx, attachments []*Attachment, insert string) error {
	for _, attachment := range attachments {
		payload := "{}"
		if object := attachment.Payload.GetS3Object(); object != nil {
			data, err := protojson.Marshal(&storepb.AttachmentPayload{
				Payload: &storepb.AttachmentPayload_S3Object_{S3Object: object},
			})
			if err != nil {
				return errors.Wrap(err, "failed to encode cleanup payload")
			}
			payload = string(data)
		}
		if _, err := tx.ExecContext(ctx, insert, attachment.ID, attachment.UID, attachment.CreatorID, attachment.MemoID,
			attachment.StorageType.String(), attachment.Reference, payload); err != nil {
			return err
		}
	}
	return nil
}

// ListMemoTrashAttachmentCleanup returns a page of jobs ready for an attempt.
func (s *Store) ListMemoTrashAttachmentCleanup(ctx context.Context, afterID int32, now time.Time, limit int) ([]*Attachment, error) {
	return s.driver.ListMemoTrashAttachmentCleanup(ctx, afterID, now.Unix(), limit)
}

// RunMemoTrashAttachmentCleanup leases an attachment job and attempts it.
// The expired lease makes an interrupted job eligible after a restart.
func (s *Store) RunMemoTrashAttachmentCleanup(ctx context.Context, attachment *Attachment, now time.Time) error {
	if current := time.Now(); current.After(now) {
		now = current
	}
	retryAt := now.Add(5 * time.Minute).Unix()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	claimed, err := s.driver.ClaimMemoTrashAttachmentCleanup(ctx, attachment.ID, now.Unix(), retryAt)
	if err != nil || !claimed {
		return err
	}
	storageSetting, err := s.getAttachmentStorageCleanupInstanceSetting(ctx, []*Attachment{attachment})
	if err != nil {
		return err
	}
	referenced, err := s.memoTrashAttachmentStorageReferenced(ctx, attachment, storageSetting)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if referenced {
		// Old attachments can share a file. Only remove the purged UID's
		// derived assets when another database record still needs that file.
		s.DeleteAttachmentDerivedCaches(attachment)
	} else if err := s.DeleteAttachmentStorageWithInstanceSetting(ctx, attachment, storageSetting); err != nil {
		return err
	}
	return s.driver.CompleteMemoTrashAttachmentCleanup(ctx, attachment.ID, retryAt)
}

func (s *Store) memoTrashAttachmentStorageReferenced(ctx context.Context, attachment *Attachment, setting *storepb.InstanceStorageSetting) (bool, error) {
	switch attachment.StorageType {
	case storepb.AttachmentStorageType_LOCAL:
		return s.localAttachmentStorageReferenced(ctx, attachment.Reference, 0, false)
	case storepb.AttachmentStorageType_S3:
		return s.s3AttachmentStorageReferenced(ctx, attachment.Payload.GetS3Object(), setting)
	default:
		return false, nil
	}
}

func (s *Store) s3AttachmentStorageReferenced(ctx context.Context, object *storepb.AttachmentPayload_S3Object, setting *storepb.InstanceStorageSetting) (bool, error) {
	if object == nil || object.Key == "" {
		return false, errors.New("missing S3 object key")
	}
	matches, err := s.driver.ListS3AttachmentStorageMatches(ctx, object.Key)
	if err != nil || len(matches) == 0 {
		return false, err
	}
	removed, err := ResolveStorage(setting, object.StorageId, object.S3Config)
	if err != nil {
		return false, err
	}
	for _, match := range matches {
		candidate := match.Payload.GetS3Object()
		if candidate.GetKey() != object.Key {
			continue
		}
		storage, err := ResolveStorage(setting, candidate.StorageId, candidate.S3Config)
		if err != nil {
			return false, err
		}
		if StorageNamespaceEqual(removed, storage) {
			return true, nil
		}
	}
	return false, nil
}

// LocalAttachmentStorageReserved checks whether a managed path belongs to an
// attachment or is awaiting cleanup. A fixed-path upload may reuse the
// missing file of the row it is replacing, but must respect other rows and
// cleanup reservations.
func (s *Store) LocalAttachmentStorageReserved(ctx context.Context, path string, replacingID int32) (bool, error) {
	return s.localAttachmentStorageReferenced(ctx, path, replacingID, true)
}

func (s *Store) localAttachmentStorageReferenced(ctx context.Context, reference string, replacingID int32, includePending bool) (bool, error) {
	if reference == "" {
		return false, errors.New("missing attachment path")
	}
	dataDir, err := filepath.Abs(s.profile.Data)
	if err != nil {
		return false, errors.Wrap(err, "failed to resolve attachment data directory")
	}
	path := cleanupAttachmentPath(dataDir, reference)
	basename := filepath.Base(path)
	suffixPattern := "%/" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(basename)
	rows, err := s.driver.ListLocalAttachmentStorageMatches(ctx, basename, suffixPattern, replacingID, includePending)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var candidate string
		if err := rows.Scan(&candidate); err != nil {
			return false, err
		}
		if path == cleanupAttachmentPath(dataDir, candidate) {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Legacy absolute paths may contain redundant separators or have been saved
// through a directory symlink. Resolve the directory without following the
// final entry, which is itself the target of unlink.
func cleanupAttachmentPath(dataDir, reference string) string {
	path := filepath.FromSlash(reference)
	if !filepath.IsAbs(path) {
		path = filepath.Join(dataDir, path)
	}
	dir, name := filepath.Split(path)
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return filepath.Join(resolved, name)
	}
	return filepath.Clean(path)
}
