package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// EnqueueAttachmentStorageCleanup saves deleted attachment metadata in the
// deletion transaction so file removal can be retried after commit.
func EnqueueAttachmentStorageCleanup(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, attachments []*Attachment, insert string) error {
	for _, attachment := range attachments {
		if _, err := tx.ExecContext(ctx, insert, attachment.ID, attachment.UID, attachment.CreatorID, attachment.MemoID,
			attachment.StorageType.String(), attachment.Reference, "{}"); err != nil {
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
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.DeleteAttachmentStorage(ctx, attachment); err != nil {
		return err
	}
	return s.driver.CompleteMemoTrashAttachmentCleanup(ctx, attachment.ID, retryAt)
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
