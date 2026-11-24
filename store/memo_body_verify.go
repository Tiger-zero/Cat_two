package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"

	"github.com/pkg/errors"
)

// MemoBodyReference identifies a memo and its committed Markdown digest.
type MemoBodyReference struct {
	ID     int32
	Digest string
}

// CheckMemoBodies checks each file referenced by SQLite. Repair can rebuild an
// absent or damaged file only from a copy that matches the committed digest.
// The caller must have exclusive access to the instance throughout the check.
func (s *Store) CheckMemoBodies(ctx context.Context, repair bool, report func(id int32, err error, repaired bool)) error {
	driver, ok := s.driver.(memoBodyDriver)
	if !ok || !s.memoBodiesEnabled() {
		return errors.New("checking Markdown files requires a Cat_two SQLite instance")
	}
	var notes *os.Root
	defer func() {
		if notes != nil {
			_ = notes.Close()
		}
	}()
	var afterID int32
	for {
		rows, err := driver.ListMemoBodyReferences(ctx, afterID, 128)
		if err != nil {
			return errors.Wrap(err, "list Markdown references")
		}
		if len(rows) == 0 {
			return nil
		}
		if notes == nil {
			notes, err = s.openMemoBodyDirectory(ctx, repair)
			if err != nil {
				return err
			}
		}
		for _, row := range rows {
			if err := ctx.Err(); err != nil {
				return err
			}
			fileErr := verifyMemoBodyFile(notes, row.Digest)
			repaired := false
			if repair && (errors.Is(fileErr, os.ErrNotExist) || errors.Is(fileErr, errMemoBodyDigestMismatch)) {
				projection, err := driver.GetMemoBodyProjection(ctx, row.ID, row.Digest)
				switch {
				case err != nil:
					fileErr = errors.Wrap(err, "read database copy for repair")
				case memoBodyDigest([]byte(projection)) == row.Digest:
					fileErr = writeMemoBody(notes, row.Digest, projection)
					repaired = fileErr == nil
				default:
					fileErr = errors.Wrap(fileErr, "SQL copy does not match the committed digest")
				}
			}
			report(row.ID, fileErr, repaired)
			afterID = row.ID
		}
	}
}

func verifyMemoBodyFile(notes *os.Root, digest string) error {
	file, err := openMemoBodyFile(notes, digest, -1)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != digest {
		return errMemoBodyDigestMismatch
	}
	return nil
}
