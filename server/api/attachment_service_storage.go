package api

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/usememos/memos/internal/random"
	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

func convertAttachmentFromStore(attachment *store.Attachment) *apipb.Attachment {
	attachmentMessage := &apipb.Attachment{
		Name:          fmt.Sprintf("%s%s", AttachmentNamePrefix, attachment.UID),
		CreateTime:    timestamppb.New(time.Unix(attachment.CreatedTs, 0)),
		Filename:      attachment.Filename,
		Type:          attachment.Type,
		Size:          attachment.Size,
		MotionMedia:   convertMotionMediaFromStore(getAttachmentMotionMedia(attachment)),
		MediaMetadata: convertMediaMetadataFromStore(attachment.Payload.GetMediaMetadata()),
	}
	if attachment.CreatorUsername != nil {
		attachmentMessage.Creator = BuildUserName(*attachment.CreatorUsername)
	}
	if attachment.MemoUID != nil && *attachment.MemoUID != "" {
		memoName := fmt.Sprintf("%s%s", MemoNamePrefix, *attachment.MemoUID)
		attachmentMessage.Memo = &memoName
	}
	// Managed files are always addressed through the authenticated file route.
	if attachment.StorageType == storepb.AttachmentStorageType_EXTERNAL {
		attachmentMessage.ExternalLink = attachment.Reference
	}

	return attachmentMessage
}

// saveAttachmentContent stages a file under the data directory and records its
// relative path after publication.
func saveAttachmentContent(ctx context.Context, dataDir string, create *store.Attachment, content io.Reader) error {
	if !validateFilename(create.Filename) {
		return errors.New("invalid attachment filename")
	}

	assets, err := store.OpenAttachmentDirectory(dataDir)
	if err != nil {
		return err
	}
	defer assets.Close()
	staged, _, err := store.StageAttachmentFile(ctx, assets, content)
	if err != nil {
		return err
	}
	defer assets.Remove(staged)
	if err := ctx.Err(); err != nil {
		return errors.Wrap(err, "failed to finalize attachment file")
	}

	const maxFilenameBytes = 255
	fileID := random.UUID()
	filename := fmt.Sprintf("%d_%s_%s", time.Now().Unix(), fileID, create.Filename)
	if len(filename) > maxFilenameBytes {
		filename = fileID
	}
	if err := publishLocalAttachment(assets, staged, filename); err != nil {
		return err
	}
	create.Reference = store.AttachmentDirectory + "/" + filename
	create.Blob = nil
	create.StorageType = storepb.AttachmentStorageType_LOCAL
	return nil
}

func cleanupSavedAttachmentFile(ctx context.Context, stores *store.Store, attachment *store.Attachment) (bool, error) {
	if attachment == nil || attachment.StorageType != storepb.AttachmentStorageType_LOCAL {
		return false, nil
	}

	cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancelCleanup()
	persisted, err := stores.GetAttachment(cleanupCtx, &store.FindAttachment{UID: &attachment.UID})
	if err != nil {
		return false, errors.Wrap(err, "failed to verify attachment create outcome")
	}
	if hasSameManagedStorageObject(stores.GetDataDir(), persisted, attachment) {
		return true, nil
	}
	return false, stores.DeleteAttachmentStorage(cleanupCtx, attachment)
}

func hasSameManagedStorageObject(dataDir string, left, right *store.Attachment) bool {
	return left != nil && right != nil &&
		left.StorageType == storepb.AttachmentStorageType_LOCAL && right.StorageType == storepb.AttachmentStorageType_LOCAL &&
		left.Reference != "" && right.Reference != "" &&
		localAttachmentPath(dataDir, left.Reference) == localAttachmentPath(dataDir, right.Reference)
}

func localAttachmentPath(dataDir, reference string) string {
	path := filepath.FromSlash(reference)
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(dataDir, path)
}

func (s *APIService) cleanupDeletedAttachmentStorage(ctx context.Context, attachments []*store.Attachment) error {
	var firstErr error
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		if err := s.Store.DeleteAttachmentStorage(ctx, attachment); err != nil && firstErr == nil {
			firstErr = errors.Wrapf(err, "attachment %d", attachment.ID)
		}
	}
	return firstErr
}

// GetAttachmentBlob reads the content of a local attachment.
func (s *APIService) GetAttachmentBlob(ctx context.Context, attachment *store.Attachment) ([]byte, error) {
	content, err := s.openAttachmentContent(ctx, attachment)
	if err != nil {
		return nil, err
	}
	defer content.Close()
	blob, err := io.ReadAll(content)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read attachment content")
	}
	return blob, nil
}

// openAttachmentContent streams a local attachment, including older absolute paths.
func (s *APIService) openAttachmentContent(ctx context.Context, attachment *store.Attachment) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if attachment == nil || attachment.StorageType != storepb.AttachmentStorageType_LOCAL || attachment.Reference == "" {
		return nil, errors.New("attachment file is unavailable")
	}
	file, err := os.Open(localAttachmentPath(s.Profile.Data, attachment.Reference))
	if err != nil {
		return nil, errors.Wrap(err, "failed to open attachment file")
	}
	return file, nil
}

// publishLocalAttachment gives the staged file its final name without replacing
// any existing file.
func publishLocalAttachment(assets *os.Root, staged, destination string) error {
	if err := assets.Link(staged, destination); err != nil {
		return errors.Wrap(err, "failed to publish attachment file")
	}
	if err := store.SyncAttachmentDirectory(assets); err != nil {
		assets.Remove(destination)
		return err
	}
	return nil
}

func validateFilename(filename string) bool {
	// Reject path traversal attempts and make sure no additional directories are created
	if !filepath.IsLocal(filename) || strings.ContainsAny(filename, "/\\") {
		return false
	}

	// Reject filenames starting or ending with spaces or periods
	if strings.HasPrefix(filename, " ") || strings.HasSuffix(filename, " ") ||
		strings.HasPrefix(filename, ".") || strings.HasSuffix(filename, ".") {
		return false
	}

	return true
}

func normalizeMimeType(mimeType string) (string, bool) {
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" || len(mimeType) > 255 {
		return "", false
	}

	mediaType, _, err := mime.ParseMediaType(mimeType)
	if err != nil || mediaType == "" || len(mediaType) > 255 {
		return "", false
	}

	return mediaType, true
}
