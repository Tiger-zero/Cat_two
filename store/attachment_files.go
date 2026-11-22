package store

import (
	"context"
	"io"
	"os"

	"github.com/pkg/errors"

	"github.com/usememos/memos/internal/random"
)

// AttachmentDirectory is the directory for managed attachments under the
// instance data directory.
const AttachmentDirectory = "assets"

// OpenAttachmentDirectory opens the managed directory without following a
// symlink at its location. The caller closes the returned root.
func OpenAttachmentDirectory(dataDir string) (*os.Root, error) {
	data, err := os.OpenRoot(dataDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open the attachment data directory")
	}
	defer data.Close()
	if err := data.Mkdir(AttachmentDirectory, 0o750); err != nil && !os.IsExist(err) {
		return nil, errors.Wrap(err, "failed to create the attachment directory")
	}
	info, err := data.Lstat(AttachmentDirectory)
	if err != nil {
		return nil, errors.Wrap(err, "failed to inspect the attachment directory")
	}
	if !info.IsDir() {
		return nil, errors.New("attachment directory must be a directory inside the data directory")
	}
	assets, err := data.OpenRoot(AttachmentDirectory)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open the attachment directory")
	}
	opened, err := assets.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		_ = assets.Close()
		return nil, errors.New("attachment directory changed while opening it")
	}
	if err := SyncAttachmentDirectory(data); err != nil {
		_ = assets.Close()
		return nil, errors.Wrap(err, "failed to sync the data directory")
	}
	return assets, nil
}

// StageAttachmentFile writes and syncs a private temporary file. The caller
// removes the returned file after publishing it or on subsequent error.
func StageAttachmentFile(ctx context.Context, root *os.Root, source io.Reader) (string, int64, error) {
	name := ".memos-upload-" + random.UUID()
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", 0, errors.Wrap(err, "failed to stage attachment")
	}
	defer file.Close()
	staged := false
	defer func() {
		if !staged {
			_ = root.Remove(name)
		}
	}()

	written, err := io.Copy(file, &attachmentFileReader{ctx: ctx, reader: source})
	if err != nil {
		return "", 0, errors.Wrap(err, "failed to copy attachment content")
	}
	if err := file.Sync(); err != nil {
		return "", 0, errors.Wrap(err, "failed to sync staged attachment")
	}
	if err := file.Close(); err != nil {
		return "", 0, errors.Wrap(err, "failed to close staged attachment")
	}
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}
	staged = true
	return name, written, nil
}

// SyncAttachmentDirectory persists directory entry changes before a database
// row can start referencing a newly published file.
func SyncAttachmentDirectory(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

type attachmentFileReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *attachmentFileReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
