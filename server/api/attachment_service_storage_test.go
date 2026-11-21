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
	instanceProfile := &profile.Profile{Data: t.TempDir()}
	setting := localAttachmentTestSetting("assets/{filename}")
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
			results <- saveAttachmentContent(ctx, instanceProfile, nil, &attachments[i], setting, reader)
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
	require.Contains(t, references, "assets/image.jpg")
	require.NotEqual(t, references[0], references[1])
	for i, attachment := range attachments {
		require.False(t, filepath.IsAbs(attachment.Reference))
		content, err := os.ReadFile(filepath.Join(instanceProfile.Data, filepath.FromSlash(attachment.Reference)))
		require.NoError(t, err)
		require.Equal(t, contents[i], string(content))
	}
	files, err := os.ReadDir(filepath.Join(instanceProfile.Data, "assets"))
	require.NoError(t, err)
	require.Len(t, files, len(attachments))
}

func TestSaveLocalAttachmentReferenceSurvivesDataDirectoryRelocation(t *testing.T) {
	ctx := t.Context()
	dataDir, newDataDir := t.TempDir(), t.TempDir()
	attachment := &store.Attachment{UID: "portable", Filename: "note.txt"}
	setting := localAttachmentTestSetting("assets")
	err := saveAttachmentContent(ctx, &profile.Profile{Data: dataDir}, nil, attachment, setting, strings.NewReader("test content"))
	require.NoError(t, err)
	require.Equal(t, "assets/note.txt", attachment.Reference)
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
	err := saveAttachmentContent(ctx, &profile.Profile{Data: dataDir}, nil, attachment, localAttachmentTestSetting("assets"), reader)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, attachment.Reference)
	files, err := os.ReadDir(filepath.Join(dataDir, "assets"))
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestPublishLocalAttachmentFailureRemovesReservation(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "note.txt")
	require.NoError(t, os.WriteFile(destination, []byte("original"), 0o600))
	_, err := publishLocalAttachment(filepath.Join(dir, "missing-staged-file"), destination, "new-uid")
	require.Error(t, err)
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	content, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, "original", string(content))
}

func localAttachmentTestSetting(template string) *storepb.InstanceStorageSetting {
	return &storepb.InstanceStorageSetting{
		FilepathTemplate: template,
		Storages:         []*storepb.Storage{{Id: "local", Type: storepb.StorageType_STORAGE_TYPE_LOCAL}},
		DefaultStorageId: "local",
	}
}

type attachmentReadFunc func([]byte) (int, error)

func (f attachmentReadFunc) Read(buffer []byte) (int, error) {
	return f(buffer)
}
