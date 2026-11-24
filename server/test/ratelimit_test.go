package test

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/proto"

	"github.com/usememos/memos/proto/gen/api/apiconnect"
)

type connectErrorResponse struct {
	Code    string `json:"code"`
	Details []struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"details"`
}

func requireConnectErrorInfo(t *testing.T, body []byte, code string) *errdetails.ErrorInfo {
	t.Helper()
	var response connectErrorResponse
	require.NoError(t, json.Unmarshal(body, &response))
	require.Equal(t, code, response.Code)
	for _, detail := range response.Details {
		if detail.Type == "google.rpc.ErrorInfo" {
			value, err := base64.RawStdEncoding.DecodeString(detail.Value)
			require.NoError(t, err)
			var info errdetails.ErrorInfo
			require.NoError(t, proto.Unmarshal(value, &info))
			return &info
		}
	}
	t.Fatalf("missing google.rpc.ErrorInfo in %s", body)
	return nil
}

func TestStartupConnectSignInRateLimit(t *testing.T) {
	inst := bootInstance(t.Context(), t, instanceOptions{instanceURL: "http://127.0.0.1", rateLimit: true})
	inst.createAdmin(t)

	// Distinct usernames isolate the address budget from any per-account budget.
	post := func(attempt int, forwardedFor string) *http.Response {
		t.Helper()
		req := inst.connectRequest(t, apiconnect.AuthServiceSignInProcedure, "", map[string]any{
			"passwordCredentials": map[string]string{"username": "nobody-" + strconv.Itoa(attempt), "password": "wrong"},
		})
		if forwardedFor != "" {
			req.Header.Set("X-Forwarded-For", forwardedFor)
		}
		resp, err := inst.client.Do(req)
		require.NoError(t, err)
		return resp
	}

	var refused *http.Response
	for attempt := range 40 {
		resp := post(attempt, "")
		if resp.StatusCode == http.StatusTooManyRequests {
			refused = resp
			break
		}
		resp.Body.Close()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	}
	require.NotNil(t, refused, "the address budget never refused a login")
	defer refused.Body.Close()
	require.NotEmpty(t, refused.Header.Get("Retry-After"))
	require.Contains(t, refused.Header.Get("RateLimit"), `"signin_ip";r=0;t=`)
	require.Contains(t, refused.Header.Get("RateLimit-Policy"), `"signin_ip";q=30;w=900`)
	body, err := io.ReadAll(refused.Body)
	require.NoError(t, err)
	require.Equal(t, "RATE_LIMITED", requireConnectErrorInfo(t, body, "resource_exhausted").Reason)
	require.Contains(t, string(body), `"type":"google.rpc.RetryInfo"`)

	// An untrusted forwarding header must not reset the address budget.
	forged := post(40, "203.0.113.5")
	defer forged.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, forged.StatusCode)
}
