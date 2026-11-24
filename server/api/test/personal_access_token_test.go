package test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/store"
)

func TestCreatePersonalAccessTokenExpiration(t *testing.T) {
	ctx := context.Background()
	ts := NewTestService(t)
	defer ts.Cleanup()

	user, err := ts.CreateRegularUser(ctx, "pat-expiration")
	require.NoError(t, err)
	userCtx := ts.CreateSessionContext(ctx, user.ID)
	parent := "users/" + user.Username

	t.Run("zero days never expires", func(t *testing.T) {
		response, err := ts.Service.CreatePersonalAccessToken(userCtx, &apipb.CreatePersonalAccessTokenRequest{
			Parent:      parent,
			Description: "non-expiring token",
		})
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(response.Token, "memos_pat_"))
		require.Nil(t, response.PersonalAccessToken.ExpireTime)

		listed, err := ts.Service.ListPersonalAccessTokens(userCtx, &apipb.ListPersonalAccessTokensRequest{Parent: parent})
		require.NoError(t, err)
		require.Len(t, listed.PersonalAccessTokens, 1)
		require.Nil(t, listed.PersonalAccessTokens[0].ExpireTime)
	})

	t.Run("positive days sets expiration", func(t *testing.T) {
		before := time.Now().Add(29 * 24 * time.Hour)
		response, err := ts.Service.CreatePersonalAccessToken(userCtx, &apipb.CreatePersonalAccessTokenRequest{
			Parent:        parent,
			Description:   "expiring token",
			ExpiresInDays: 30,
		})
		require.NoError(t, err)
		require.NotNil(t, response.PersonalAccessToken.ExpireTime)
		require.True(t, response.PersonalAccessToken.ExpireTime.AsTime().After(before))
		require.True(t, response.PersonalAccessToken.ExpireTime.AsTime().Before(time.Now().Add(31*24*time.Hour)))
	})
}

func TestPersonalAccessTokenManagementRequiresBrowserSession(t *testing.T) {
	ctx := t.Context()
	ts := NewTestService(t)
	t.Cleanup(ts.Cleanup)
	const password = "token management password"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	require.NoError(t, err)
	_, err = ts.Store.CreateUser(ctx, &store.User{Username: "pat-owner", Role: store.RoleAdmin, PasswordHash: string(hash)})
	require.NoError(t, err)
	handler, err := ts.Service.NewHandler(ctx)
	require.NoError(t, err)

	call := func(method, path, token, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}

	response := call(http.MethodPost, "/memos.api.AuthService/SignIn", "",
		`{"passwordCredentials":{"username":"pat-owner","password":"token management password"}}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var session struct {
		AccessToken string `json:"accessToken"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &session))
	require.NotEmpty(t, session.AccessToken)

	const tokensPath = "/api/users/pat-owner/personalAccessTokens"
	response = call(http.MethodPost, tokensPath, session.AccessToken, `{"description":"integration client","expiresInDays":7}`)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var created struct {
		Token               string `json:"token"`
		PersonalAccessToken struct {
			Name string `json:"name"`
		} `json:"personalAccessToken"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &created))
	require.NotEmpty(t, created.Token)
	require.NotEmpty(t, created.PersonalAccessToken.Name)

	response = call(http.MethodGet, "/api/auth/me", created.Token, "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	deletePath := "/api/" + created.PersonalAccessToken.Name
	response = call(http.MethodPost, tokensPath, created.Token, `{"description":"another token"}`)
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	response = call(http.MethodGet, tokensPath, created.Token, "")
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	response = call(http.MethodDelete, deletePath, created.Token, "")
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	response = call(http.MethodPost, "/memos.api.UserService/CreatePersonalAccessToken", created.Token,
		`{"parent":"users/pat-owner","description":"another token"}`)
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())

	response = call(http.MethodGet, tokensPath, session.AccessToken, "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	response = call(http.MethodDelete, deletePath, session.AccessToken, "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	response = call(http.MethodGet, "/api/auth/me", created.Token, "")
	require.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
}
