package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestRemovedAPIVersionRoutesReturnGone(t *testing.T) {
	svc, _ := newUploadTestService(t)
	e := http.NewServeMux()
	apiHandler, handlerErr := svc.NewHandler(context.Background())
	require.NoError(t, handlerErr)
	e.Handle("/", apiHandler)

	tests := []struct {
		method   string
		path     string
		wantCode any
	}{
		{http.MethodGet, "/api/v1/memos", float64(12)},
		{http.MethodPost, "/api/v1/auth/signin", float64(12)},
		{http.MethodPost, "/memos.api.v1.MemoService/ListMemos", "unimplemented"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader("{}"))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			require.Equal(t, http.StatusGone, rec.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, tt.wantCode, body["code"])
			require.Equal(t, removedAPIMessage, body["message"])
		})
	}

	// The current routes are unaffected.
	req := httptest.NewRequest(http.MethodGet, "/api/instance/profile", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestMCPIdentityRequiresMatchingAPIProcedure(t *testing.T) {
	svc, _ := newUploadTestService(t)
	handler, err := svc.NewHandler(context.Background())
	require.NoError(t, err)

	user := &store.User{ID: 1}
	ctx := WithMCPRequest(context.Background(), user, "/memos.api.MemoService/ListMemos")
	request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/users", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "MCP connection cannot access this API procedure")
}
