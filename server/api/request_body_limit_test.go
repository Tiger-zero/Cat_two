package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Only procedures that carry file bytes inline keep the large body cap; every
// other request is bounded by the general cap so buffered bodies cannot be
// used to exhaust memory.
func TestRequestBodyLimitPerProcedure(t *testing.T) {
	require.EqualValues(t, uploadRequestLimit, requestBodyLimit(attachmentUploadProcedure))
	require.EqualValues(t, uploadRequestLimit, requestBodyLimit(importMemosProcedure))
	require.EqualValues(t, MaxAPIRequestBytes, requestBodyLimit("/memos.api.AttachmentService/CreateAttachment"))
	for _, procedure := range []string{
		"/memos.api.AuthService/SignIn",
		"/memos.api.UserService/CreateUser",
		"/memos.api.AuthService/RefreshToken",
	} {
		require.EqualValues(t, maxAuthRequestBytes, requestBodyLimit(procedure), procedure)
	}
	for _, procedure := range []string{
		"/memos.api.MemoService/CreateMemo",
		"/memos.api.MemoService/ListMemos",
	} {
		require.EqualValues(t, maxGeneralRequestBytes, requestBodyLimit(procedure), procedure)
	}
	require.Less(t, int64(maxGeneralRequestBytes), int64(MaxAPIRequestBytes))
}

func TestConnectAuthSubmissionBodyLimit(t *testing.T) {
	service, _ := newUploadTestService(t)
	handler := service.NewHandler()
	for _, route := range []string{
		"/memos.api.AuthService/SignIn",
		"/memos.api.UserService/CreateUser",
	} {
		t.Run(route, func(t *testing.T) {
			data := `{"padding":"` + strings.Repeat("a", maxAuthRequestBytes) + `"}`
			reader := &uploadCountingReader{reader: strings.NewReader(data)}
			request := httptest.NewRequest(http.MethodPost, route, reader)
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.GreaterOrEqual(t, response.Code, 400, response.Body.String())
			require.Equal(t, maxAuthRequestBytes+1, reader.read, "transport must bound the request before decoding")
		})
	}
}
