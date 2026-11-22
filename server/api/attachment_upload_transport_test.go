package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/proto/gen/api/apiconnect"
	"github.com/usememos/memos/server/auth"
)

type uploadCountingReader struct {
	reader io.Reader
	read   int
}

func (r *uploadCountingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += n
	return n, err
}

func TestCreateAttachmentConnectChecksCredentialsBeforeReadingBody(t *testing.T) {
	svc, ctx := newUploadTestService(t)
	handler, err := svc.NewHandler(t.Context())
	require.NoError(t, err)
	attachment := &apipb.CreateAttachmentRequest{Attachment: &apipb.Attachment{
		Filename: "inline.txt",
		Type:     "text/plain",
		Content:  []byte("sample attachment"),
	}}
	data, err := protojson.Marshal(attachment)
	require.NoError(t, err)
	for _, tc := range []struct {
		name, contentType, credential string
		status                        int
	}{
		{name: "anonymous", contentType: "application/json", status: http.StatusUnauthorized},
		{name: "invalid credential", contentType: "application/json; charset=UTF-8", credential: "Bearer invalid", status: http.StatusUnauthorized},
		{name: "unsupported media type", contentType: "application/xml", status: http.StatusUnsupportedMediaType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &uploadCountingReader{reader: bytes.NewReader(data)}
			request := httptest.NewRequest(http.MethodPost, apiconnect.AttachmentServiceCreateAttachmentProcedure, reader)
			request.Header.Set("Content-Type", tc.contentType)
			request.Header.Set("Authorization", tc.credential)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			require.Zero(t, reader.read, "refused requests must not reach the body decoder")
			if tc.status == http.StatusUnauthorized {
				require.Contains(t, response.Body.String(), `"code":"unauthenticated"`)
				require.Contains(t, response.Header().Get("Cache-Control"), "no-store")
			}
		})
	}

	user, err := svc.fetchCurrentUser(ctx)
	require.NoError(t, err)
	token, _, err := auth.GenerateAccessTokenV2(user.ID, user.Username, string(user.Role), string(user.RowStatus), user.PasswordHash, []byte(svc.Secret))
	require.NoError(t, err)
	server := httptest.NewTestServer(t, handler)
	client := apiconnect.NewAttachmentServiceClient(server.Client(), server.URL)
	request := connect.NewRequest(attachment)
	request.Header().Set("Authorization", "Bearer "+token)
	response, err := client.CreateAttachment(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, attachment.Attachment.Filename, response.Msg.Filename)
	require.EqualValues(t, len(attachment.Attachment.Content), response.Msg.Size)
}

func TestUploadAttachmentTransports(t *testing.T) {
	svc, ctx := newUploadTestService(t)
	user, err := svc.fetchCurrentUser(ctx)
	require.NoError(t, err)
	token, _, err := auth.GenerateAccessTokenV2(user.ID, user.Username, string(user.Role), string(user.RowStatus), user.PasswordHash, []byte(svc.Secret))
	require.NoError(t, err)
	e := http.NewServeMux()
	apiHandler, handlerErr := svc.NewHandler(context.Background())
	require.NoError(t, handlerErr)
	e.Handle("/", apiHandler)

	for _, path := range []string{"/api/attachments:upload", attachmentUploadProcedure} {
		t.Run(path, func(t *testing.T) {
			call := func(message proto.Message, authenticated bool) *httptest.ResponseRecorder {
				t.Helper()
				data, err := protojson.Marshal(message)
				require.NoError(t, err)
				req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
				req.Header.Set("Content-Type", "application/json")
				if authenticated {
					req.Header.Set("Authorization", "Bearer "+token)
				}
				rec := httptest.NewRecorder()
				e.ServeHTTP(rec, req)
				return rec
			}
			initial := &apipb.UploadAttachmentRequest{Upload: uploadSpec("file.txt", 3)}
			require.Equal(t, http.StatusUnauthorized, call(initial, false).Code)
			rec := call(initial, true)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			response := &apipb.UploadAttachmentResponse{}
			require.NoError(t, protojson.Unmarshal(rec.Body.Bytes(), response))
			rec = call(&apipb.UploadAttachmentRequest{Upload: uploadID(response.UploadId), Data: []byte("abc"), FinishWrite: true}, true)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			require.NoError(t, protojson.Unmarshal(rec.Body.Bytes(), response))
			require.NotNil(t, response.Attachment)

			oversized, err := protojson.Marshal(&apipb.UploadAttachmentRequest{Data: make([]byte, uploadRequestLimit)})
			require.NoError(t, err)
			reader := &uploadCountingReader{reader: bytes.NewReader(oversized)}
			req := httptest.NewRequest(http.MethodPost, path, reader)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			rec = httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			require.GreaterOrEqual(t, rec.Code, 400)
			require.LessOrEqual(t, reader.read, uploadRequestLimit+1, "body must be bounded before decoding")
		})
	}

	t.Run("Connect protobuf and compressed message limit", func(t *testing.T) {
		server := httptest.NewTestServer(t, e)
		client := apiconnect.NewAttachmentServiceClient(server.Client(), server.URL)
		initial := connect.NewRequest(&apipb.UploadAttachmentRequest{Upload: uploadSpec("binary.bin", uploadChunkSize)})
		initial.Header().Set("Authorization", "Bearer "+token)
		response, err := client.UploadAttachment(context.Background(), initial)
		require.NoError(t, err)
		last := connect.NewRequest(&apipb.UploadAttachmentRequest{Upload: uploadID(response.Msg.UploadId), Data: make([]byte, uploadChunkSize), FinishWrite: true})
		last.Header().Set("Authorization", "Bearer "+token)
		finished, err := client.UploadAttachment(context.Background(), last)
		require.NoError(t, err)
		require.EqualValues(t, uploadChunkSize, finished.Msg.Attachment.Size)

		data, err := proto.Marshal(&apipb.UploadAttachmentRequest{Data: make([]byte, uploadRequestLimit+1)})
		require.NoError(t, err)
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		_, err = writer.Write(data)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		req := httptest.NewRequest(http.MethodPost, attachmentUploadProcedure, &compressed)
		req.Header.Set("Content-Type", "application/proto")
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
		require.Contains(t, rec.Body.String(), "resource_exhausted")
	})
}
