package test

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/server/auth"
)

func TestSSEHandler_Authentication(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "sse-user")
	require.NoError(t, err)

	token, _, err := auth.GenerateAccessTokenV2(
		user.ID,
		user.Username,
		string(user.Role),
		string(user.RowStatus),
		user.PasswordHash,
		[]byte(ts.Secret),
	)
	require.NoError(t, err)

	e := http.NewServeMux()
	api.RegisterSSERoutes(e, ts.Service.SSEHub, ts.Store, ts.Secret)

	t.Run("no token returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/internal/sse", nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/internal/sse", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("valid token returns 200 and stream", func(t *testing.T) {
		// Wait for the flushed response before disconnecting so authentication
		// can finish its database reads.
		server := httptest.NewTestServer(t, e)
		client := server.Client()
		reqCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, server.URL+"/internal/sse", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
	})

	t.Run("token in query param returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/internal/sse?token="+token, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("valid token streams initial comment and event", func(t *testing.T) {
		server := httptest.NewTestServer(t, e)
		client := server.Client()

		reqCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, server.URL+"/internal/sse", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

		reader := bufio.NewReader(resp.Body)
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, ": connected\n", line)
		line, err = reader.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, "\n", line)

		_, err = ts.Service.CreateMemo(ts.CreateUserContext(ctx, user.ID), &apipb.CreateMemoRequest{
			MemoId: "streamed",
			Memo: &apipb.Memo{
				Content:    "streamed memo",
				Visibility: apipb.Visibility_PUBLIC,
			},
		})
		require.NoError(t, err)

		line, err = reader.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, "data: {\"type\":\"memo.changed\"}\n", line)
		line, err = reader.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, "\n", line)
	})

	t.Run("revoked session stops streaming before the next event", func(t *testing.T) {
		const sessionID = "sse-session"
		require.NoError(t, ts.Store.AddUserRefreshToken(ctx, user.ID, &storepb.RefreshTokensUserSetting_RefreshToken{
			TokenId: "refresh-credential", SessionId: sessionID,
		}))
		token, _, err := auth.GenerateSessionAccessToken(user, sessionID, "", []byte(ts.Secret))
		require.NoError(t, err)
		server := httptest.NewTestServer(t, e)
		client := server.Client()
		requestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, server.URL+"/internal/sse", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		reader := bufio.NewReader(resp.Body)
		line, err := reader.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, ": connected\n", line)
		line, err = reader.ReadString('\n')
		require.NoError(t, err)
		require.Equal(t, "\n", line)

		require.NoError(t, ts.Store.RemoveUserRefreshToken(ctx, user.ID, sessionID))
		_, err = ts.Service.CreateMemo(ts.CreateUserContext(ctx, user.ID), &apipb.CreateMemoRequest{
			MemoId: "after-sign-out", Memo: &apipb.Memo{Content: "saved after sign-out"},
		})
		require.NoError(t, err)
		line, err = reader.ReadString('\n')
		require.ErrorIs(t, err, io.EOF)
		require.Empty(t, line)
	})

	t.Run("hub close disconnects stream", func(t *testing.T) {
		server := httptest.NewTestServer(t, e)
		client := server.Client()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/internal/sse", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req) //nolint:bodyclose // Body is closed after verifying the SSE stream disconnects.
		if err != nil {
			t.Fatal(err)
		}
		body := resp.Body
		defer body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

		ts.Service.SSEHub.Close()

		done := make(chan error, 1)
		go func() {
			_, err := io.ReadAll(body)
			done <- err
		}()

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(time.Second):
			t.Fatal("SSE stream did not close after hub close")
		}
	})
}
