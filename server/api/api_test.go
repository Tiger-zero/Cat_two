package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/proto/gen/api/apiconnect"
	"github.com/usememos/memos/server/auth"
)

func TestRetiredRESTRoutesReturnNotFound(t *testing.T) {
	svc, ctx := newUploadTestService(t)
	handler := svc.NewHandler()
	user, err := svc.fetchCurrentUser(ctx)
	require.NoError(t, err)
	token, _, err := auth.GenerateAccessTokenV2(user.ID, user.Username, string(user.Role), string(user.RowStatus), user.PasswordHash, []byte(svc.Secret))
	require.NoError(t, err)
	for _, route := range []struct {
		method, path string
	}{
		{http.MethodGet, "/api"},
		{http.MethodGet, "/api/instance/profile"},
		{http.MethodPost, "/api/auth/signin"},
		{http.MethodPost, "/api/memos"},
		{http.MethodGet, "/api/sse"},
		{http.MethodGet, "/api/v1/memos"},
		{http.MethodPost, "/memos.api.v1.MemoService/ListMemos"},
	} {
		t.Run(route.path, func(t *testing.T) {
			for _, credential := range []string{"", "Bearer " + token} {
				reader := &uploadCountingReader{reader: strings.NewReader("{}")}
				req := httptest.NewRequest(route.method, route.path, reader)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", credential)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
				require.Zero(t, reader.read, "removed routes cannot consume request data")
			}
		})
	}

	req := httptest.NewRequest(http.MethodPost, apiconnect.InstanceServiceGetInstanceProfileProcedure, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestInternalRoutesRequireWebSessionAndAcceptSameOrigin(t *testing.T) {
	svc, ctx := newUploadTestService(t)
	handler := svc.NewHandler()
	user, err := svc.fetchCurrentUser(ctx)
	require.NoError(t, err)
	token, _, err := auth.GenerateAccessTokenV2(user.ID, user.Username, string(user.Role), string(user.RowStatus), user.PasswordHash, []byte(svc.Secret))
	require.NoError(t, err)

	for _, path := range []string{"/internal/auth/totp", "/internal/tags/normalize", "/internal/trash"} {
		t.Run(path, func(t *testing.T) {
			method := http.MethodGet
			if strings.HasPrefix(path, "/internal/tags/") {
				method = http.MethodPost
			}
			for _, tc := range []struct {
				token, origin string
				code          int
			}{
				{origin: "http://example.com", code: http.StatusUnauthorized},
				{token: token, origin: "http://foreign.example", code: http.StatusForbidden},
				{token: token, origin: "http://example.com", code: http.StatusOK},
			} {
				req := httptest.NewRequest(method, path, nil)
				req.Header.Set("Origin", tc.origin)
				if tc.token != "" {
					req.Header.Set("Authorization", "Bearer "+tc.token)
				}
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				require.Equal(t, tc.code, rec.Code, rec.Body.String())
				require.Contains(t, rec.Header().Get("Cache-Control"), "no-store")
			}
		})
	}
}
