package mcp

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/internal/clientip"
	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/ratelimit"
)

func TestPersonalMCPTransportLimitsBeforeTokenCheck(t *testing.T) {
	service := &MCPService{
		profile: &profile.Profile{Personal: true},
		RateLimiter: ratelimit.NewMemoryLimiter(ratelimit.Policy{
			ratelimit.ScopeMCPIP: {Limit: 1, Window: time.Minute},
		}),
	}
	mux := http.NewServeMux()
	service.RegisterRoutes(mux)
	call := func(ip, credential string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		request = request.WithContext(clientip.WithClientIP(request.Context(), ip))
		request.Header.Set("Authorization", credential)
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}
	require.Equal(t, http.StatusUnauthorized, call("192.0.2.1", "").Code)
	// The service has no backing store. Exceeding the pre-authentication limit
	// stops this syntactically valid credential from reaching token lookup.
	limited := call("192.0.2.1", "Bearer cat_two_mcp_"+"test-candidate")
	require.Equal(t, http.StatusTooManyRequests, limited.Code)
	require.NotEmpty(t, limited.Header().Get("Retry-After"))
	require.Equal(t, "no-store", limited.Header().Get("Cache-Control"))
	require.Equal(t, http.StatusUnauthorized, call("192.0.2.2", "").Code)
}
