package s3

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/internal/testutil/fakes3"
	storepb "github.com/usememos/memos/proto/gen/store"
)

func TestGetObjectStream(t *testing.T) {
	ctx := context.Background()
	const bucket = "attachments"
	const key = "assets/notes/test.txt"
	content := []byte("attachment stored in fake S3")
	fake := fakes3.New(t, bucket)
	require.NoError(t, fake.PutObject(bucket, key, "text/plain", content))
	driver, err := NewDriver(ctx, fake.Config(bucket))
	require.NoError(t, err)

	stream, err := driver.GetObjectStream(ctx, key)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stream.Body.Close() })
	streamed, err := io.ReadAll(stream.Body)
	require.NoError(t, err)
	require.Equal(t, content, streamed)
	require.Equal(t, int64(len(content)), stream.ContentLength)

	missing, err := driver.GetObjectStream(ctx, "missing.txt")
	require.ErrorContains(t, err, "failed to get object")
	require.Nil(t, missing)
}

func TestNewDriverInsecureSkipTLSVerify(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/memos/note.txt", r.URL.Path)
		_, err := w.Write([]byte("stored memo"))
		require.NoError(t, err)
	}))
	defer server.Close()

	newConfig := func(skipVerify bool) *storepb.StorageS3Config {
		return &storepb.StorageS3Config{
			AccessKeyId:           "access-key",
			AccessKeySecret:       "access-secret",
			Endpoint:              server.URL,
			Region:                "us-east-1",
			Bucket:                "memos",
			UsePathStyle:          true,
			InsecureSkipTlsVerify: skipVerify,
		}
	}

	t.Run("rejects self-signed certificate by default", func(t *testing.T) {
		driver, err := NewDriver(context.Background(), newConfig(false))
		require.NoError(t, err)

		_, err = driver.GetObjectStream(context.Background(), "note.txt")
		require.Error(t, err)
		require.ErrorContains(t, err, "certificate")
	})

	t.Run("accepts self-signed certificate when enabled", func(t *testing.T) {
		driver, err := NewDriver(context.Background(), newConfig(true))
		require.NoError(t, err)

		stream, err := driver.GetObjectStream(context.Background(), "note.txt")
		require.NoError(t, err)
		t.Cleanup(func() { _ = stream.Body.Close() })
		content, err := io.ReadAll(stream.Body)
		require.NoError(t, err)
		require.Equal(t, []byte("stored memo"), content)
	})
}
