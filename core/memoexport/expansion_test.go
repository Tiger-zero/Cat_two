package memoexport

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// A container whose entries declare far more bytes than it holds is refused
// before any entry is read.
func TestReadRejectsDecompressionBomb(t *testing.T) {
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	entry, err := writer.Create(AttachmentsDir + "bomb/zeros.bin")
	require.NoError(t, err)
	zeros := make([]byte, 1<<20)
	for range 80 {
		_, err = entry.Write(zeros)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	require.Less(t, buf.Len(), 1<<20, "80 MiB of zeros deflates to well under a mebibyte")

	_, err = Read(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.Error(t, err)
	require.Contains(t, err.Error(), "uncompressed bytes")
}

func TestWriteAttachmentRoundTripsHighlyCompressibleBytes(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "export-*.zip")
	require.NoError(t, err)
	defer file.Close()

	fixture := newGoldenFixture()
	writer := NewWriter(file, fixtureExportTime)
	require.NoError(t, writer.WriteManifest(&fixture.manifest))

	// Default Deflate would make an attachment of this size exceed the
	// reader's permitted expansion factor and slack.
	const chunks = 80
	block := make([]byte, 1<<20)
	readers := make([]io.Reader, 0, chunks)
	digest := sha256.New()
	for range chunks {
		readers = append(readers, bytes.NewReader(block))
		_, err := digest.Write(block)
		require.NoError(t, err)
	}
	record := fixture.memos[1]
	attachment := &record.Attachments[0]
	attachment.SHA256, attachment.Size, err = writer.WriteAttachment(attachment.Path, io.MultiReader(readers...))
	require.NoError(t, err)
	require.EqualValues(t, chunks*len(block), attachment.Size)
	require.Equal(t, hex.EncodeToString(digest.Sum(nil)), attachment.SHA256)
	for _, memo := range fixture.memos {
		require.NoError(t, writer.WriteMemo(memo, fixture.contents[memo.UID]))
	}
	require.NoError(t, writer.Close())
	info, err := file.Stat()
	require.NoError(t, err)
	archive, err := Read(file, info.Size())
	require.NoError(t, err)
	restored, err := archive.ReadAttachment(attachment)
	require.NoError(t, err)
	require.Equal(t, attachment.Size, int64(len(restored)))
}
