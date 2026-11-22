package api

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/internal/profile"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

func TestConvertAttachmentFromStoreHidesManagedStorageReference(t *testing.T) {
	s3Attachment := &store.Attachment{
		UID:         "s3-image",
		Filename:    "image.png",
		Type:        "image/png",
		StorageType: storepb.AttachmentStorageType_S3,
		Reference:   "https://s3.example.com/presigned-secret",
	}
	require.Empty(t, convertAttachmentFromStore(s3Attachment).ExternalLink)

	externalAttachment := &store.Attachment{
		UID:         "external-image",
		Filename:    "image.png",
		Type:        "image/png",
		StorageType: storepb.AttachmentStorageType_EXTERNAL,
		Reference:   "https://cdn.example.com/image.png",
	}
	require.Equal(t, externalAttachment.Reference, convertAttachmentFromStore(externalAttachment).ExternalLink)
}

func TestSaveLocalAttachmentConcurrentSameFilename(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
	}()
	dataDir := t.TempDir()
	attachments := []store.Attachment{
		{UID: "first", Filename: "image.jpg"},
		{UID: "second", Filename: "image.jpg"},
	}
	contents := []string{"first image", "second image"}
	entered := make(chan struct{}, len(attachments))
	release := make(chan struct{})
	results := make(chan error, len(attachments))
	for i := range attachments {
		workers.Go(func() {
			content := strings.NewReader(contents[i])
			firstRead := true
			reader := attachmentReadFunc(func(buffer []byte) (int, error) {
				if firstRead {
					firstRead = false
					entered <- struct{}{}
					select {
					case <-release:
					case <-ctx.Done():
						return 0, ctx.Err()
					}
				}
				return content.Read(buffer)
			})
			results <- saveAttachmentContent(ctx, dataDir, &attachments[i], reader)
		})
	}
	for range attachments {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("both copies must be staged before publication")
		}
	}
	close(release)
	for range attachments {
		select {
		case err := <-results:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal("attachment publication timed out")
		}
	}
	references := []string{attachments[0].Reference, attachments[1].Reference}
	require.NotEqual(t, references[0], references[1])
	for i, attachment := range attachments {
		require.Regexp(t, `^assets/[0-9]+_[0-9a-f-]+_image\.jpg$`, attachment.Reference)
		require.Equal(t, storepb.AttachmentStorageType_LOCAL, attachment.StorageType)
		require.Nil(t, attachment.Blob)
		content, err := os.ReadFile(filepath.Join(dataDir, filepath.FromSlash(attachment.Reference)))
		require.NoError(t, err)
		require.Equal(t, contents[i], string(content))
	}
	files, err := os.ReadDir(filepath.Join(dataDir, store.AttachmentDirectory))
	require.NoError(t, err)
	require.Len(t, files, len(attachments))
}

func TestSaveLocalAttachmentReferenceSurvivesDataDirectoryRelocation(t *testing.T) {
	ctx := t.Context()
	dataDir, newDataDir := t.TempDir(), t.TempDir()
	attachment := &store.Attachment{UID: "portable", Filename: "note.txt"}
	err := saveAttachmentContent(ctx, dataDir, attachment, strings.NewReader("test content"))
	require.NoError(t, err)
	require.NoError(t, os.Rename(filepath.Join(dataDir, "assets"), filepath.Join(newDataDir, "assets")))

	svc := &APIService{Profile: &profile.Profile{Data: newDataDir}}
	content, err := svc.GetAttachmentBlob(ctx, attachment)
	require.NoError(t, err)
	require.Equal(t, "test content", string(content))
}

func TestSaveLocalAttachmentCancellationRemovesStagedFile(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	dataDir := t.TempDir()
	attachment := &store.Attachment{UID: "canceled", Filename: "note.txt"}
	content := strings.NewReader("complete content")
	reader := attachmentReadFunc(func(buffer []byte) (int, error) {
		n, err := content.Read(buffer)
		if err == io.EOF {
			cancel()
		}
		return n, err
	})
	err := saveAttachmentContent(ctx, dataDir, attachment, reader)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, attachment.Reference)
	files, err := os.ReadDir(filepath.Join(dataDir, "assets"))
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestSaveLocalAttachmentRejectsPathEscape(t *testing.T) {
	dataDir, outside := t.TempDir(), t.TempDir()
	content := strings.NewReader("do not write outside data")
	attachment := &store.Attachment{Filename: "../escaped.txt"}
	require.Error(t, saveAttachmentContent(t.Context(), dataDir, attachment, content))
	require.Empty(t, attachment.Reference)
	require.NoDirExists(t, filepath.Join(dataDir, store.AttachmentDirectory))

	require.NoError(t, os.Symlink(outside, filepath.Join(dataDir, store.AttachmentDirectory)))
	attachment.Filename = "valid.txt"
	require.Error(t, saveAttachmentContent(t.Context(), dataDir, attachment, content))
	require.Empty(t, attachment.Reference)
	files, err := os.ReadDir(outside)
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestPublishLocalAttachmentPreservesExistingFile(t *testing.T) {
	dir := t.TempDir()
	assets, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer assets.Close()

	const filename = "note.txt"
	destination := filepath.Join(dir, filename)
	require.NoError(t, os.WriteFile(destination, []byte("original"), 0o600))
	const staged = "staged.txt"
	require.NoError(t, assets.WriteFile(staged, []byte("new"), 0o600))
	require.ErrorIs(t, publishLocalAttachment(assets, staged, filename), os.ErrExist)
	require.ErrorIs(t, publishLocalAttachment(assets, "missing-staged-file", "new.txt"), os.ErrNotExist)
	require.NoFileExists(t, filepath.Join(dir, "new.txt"))
	content, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, "original", string(content))
}

type attachmentReadFunc func([]byte) (int, error)

func (f attachmentReadFunc) Read(buffer []byte) (int, error) {
	return f(buffer)
}
