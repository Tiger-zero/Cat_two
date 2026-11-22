package fileserver

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/internal/httphandler"
	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

// TestDrawingPNGRoundTripsUnchanged guards the drawing format of @usememos/canvas: a
// drawing is a PNG whose editable scene sits in a zTXt chunk ("memos-canvas"). Uploads
// of image/png must be stored and served byte for byte, never re-encoded or stripped
// of metadata, or "Edit drawing" would find no scene. The fixture is
// tests/fixtures/png/v1.canvas.png from usememos/canvas.
func TestDrawingPNGRoundTripsUnchanged(t *testing.T) {
	fixture, err := os.ReadFile("testdata/v1.canvas.png")
	require.NoError(t, err)
	require.True(t, bytes.Contains(fixture, []byte("zTXtmemos-canvas\x00")), "fixture carries the scene chunk")

	uploads := map[string]func(context.Context, *testing.T, *api.APIService, []byte) *apipb.Attachment{
		"inline":  createDrawingInline,
		"chunked": uploadDrawingInChunks,
	}

	for uploadName, upload := range uploads {
		t.Run(uploadName, func(t *testing.T) {
			ctx := context.Background()
			svc, fs, stores, cleanup := newShareAttachmentTestServices(ctx, t)
			defer cleanup()

			creator, err := stores.CreateUser(ctx, &store.User{Username: "artist", Role: store.RoleUser, Email: "artist@example.com"})
			require.NoError(t, err)
			creatorCtx := context.WithValue(ctx, auth.UserIDContextKey, creator.ID)

			attachment := upload(creatorCtx, t, svc, fixture)
			require.Equal(t, int64(len(fixture)), attachment.Size)
			_, err = svc.CreateMemo(creatorCtx, &apipb.CreateMemoRequest{Memo: &apipb.Memo{
				Content:     "a drawing",
				Visibility:  apipb.Visibility_PRIVATE,
				Attachments: []*apipb.Attachment{{Name: attachment.Name}},
			}})
			require.NoError(t, err)

			mux := http.NewServeMux()
			fs.RegisterRoutes(mux, http.HandlerFunc(httphandler.NotFound))
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, newAuthenticatedFileRequest(ctx, t, svc, creator.ID, fmt.Sprintf("/file/%s/%s", attachment.Name, attachment.Filename)))

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
			require.Equal(t, fixture, recorder.Body.Bytes(), "the original file is served byte for byte")
		})
	}
}

const drawingFilename = "drawing-20261106-101500.canvas.png"

func createDrawingInline(ctx context.Context, t *testing.T, svc *api.APIService, content []byte) *apipb.Attachment {
	t.Helper()
	attachment, err := svc.CreateAttachment(ctx, &apipb.CreateAttachmentRequest{Attachment: &apipb.Attachment{
		Filename: drawingFilename,
		Type:     "image/png",
		Content:  content,
	}})
	require.NoError(t, err)
	return attachment
}

// uploadDrawingInChunks uses the chunked upload the web editor uses for drafts.
func uploadDrawingInChunks(ctx context.Context, t *testing.T, svc *api.APIService, content []byte) *apipb.Attachment {
	t.Helper()
	half := len(content) / 2
	started, err := svc.UploadAttachment(ctx, &apipb.UploadAttachmentRequest{
		Upload: &apipb.UploadAttachmentRequest_Spec{Spec: &apipb.UploadAttachmentSpec{
			Attachment: &apipb.Attachment{Filename: drawingFilename, Type: "image/png"},
			TotalSize:  int64(len(content)),
		}},
		Data: content[:half],
	})
	require.NoError(t, err)
	finished, err := svc.UploadAttachment(ctx, &apipb.UploadAttachmentRequest{
		Upload:      &apipb.UploadAttachmentRequest_UploadId{UploadId: started.UploadId},
		WriteOffset: int64(half),
		Data:        content[half:],
		FinishWrite: true,
	})
	require.NoError(t, err)
	require.NotNil(t, finished.Attachment)
	return finished.Attachment
}
