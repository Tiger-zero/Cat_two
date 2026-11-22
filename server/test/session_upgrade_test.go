package test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

func TestPersonalLegacySessionUpgradesOnRefresh(t *testing.T) {
	ctx := t.Context()
	instance := bootInstance(ctx, t, instanceOptions{personal: true})
	instance.createAdmin(t)
	admin, err := instance.server.Store.GetUser(ctx, &store.FindUser{Username: new(testAdminUsername)})
	require.NoError(t, err)
	require.NotNil(t, admin)
	secret := []byte(instance.server.Secret)
	const legacyID = "pre-upgrade-browser"
	legacyAccess, _, err := auth.GenerateAccessTokenV2(admin.ID, admin.Username, string(admin.Role), string(admin.RowStatus), admin.PasswordHash, secret)
	require.NoError(t, err)
	legacyRefresh, expiry, err := auth.GenerateRefreshToken(admin.ID, legacyID, secret)
	require.NoError(t, err)
	legacyCreatedAt := time.Now().Add(-5 * time.Minute).UTC()
	require.NoError(t, instance.server.Store.AddUserRefreshToken(ctx, admin.ID, &storepb.RefreshTokensUserSetting_RefreshToken{
		TokenId: legacyID, ExpiresAt: timestamppb.New(expiry), CreatedAt: timestamppb.New(legacyCreatedAt),
	}))
	me, err := http.NewRequestWithContext(ctx, http.MethodGet, instance.baseURL+"/api/auth/me", nil)
	require.NoError(t, err)
	me.Header.Set("Authorization", "Bearer "+legacyAccess)
	me.AddCookie(&http.Cookie{Name: auth.RefreshTokenCookieName, Value: legacyRefresh})
	unauthenticated, err := instance.client.Do(me)
	require.NoError(t, err)
	defer unauthenticated.Body.Close()
	require.Equal(t, http.StatusUnauthorized, unauthenticated.StatusCode)
	require.Empty(t, unauthenticated.Cookies(), "the first unauthorized response must preserve the legacy refresh cookie")

	refresh, err := http.NewRequestWithContext(ctx, http.MethodPost, instance.baseURL+"/api/auth/refresh", strings.NewReader("{}"))
	require.NoError(t, err)
	refresh.Header.Set("Content-Type", "application/json")
	refresh.AddCookie(&http.Cookie{Name: auth.RefreshTokenCookieName, Value: legacyRefresh})
	response, err := instance.client.Do(refresh)
	require.NoError(t, err)
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode, "%s", payload)
	require.Equal(t, legacyCreatedAt.Add(store.UserSessionIdleTimeout).Format(time.RFC3339Nano), response.Header.Get("X-Memos-Session-Idle-Expires-At"))
	stored, err := instance.server.Store.GetUserRefreshTokens(ctx, admin.ID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, legacyCreatedAt, stored[0].GetLastEditAt().AsTime())
	var upgraded struct{ AccessToken string }
	require.NoError(t, json.Unmarshal(payload, &upgraded))
	claims, err := auth.ParseAccessTokenV2(upgraded.AccessToken, secret)
	require.NoError(t, err)
	require.Equal(t, legacyID, claims.SessionID)
	code, _ := instance.do(t, http.MethodGet, "/api/auth/me", upgraded.AccessToken, nil)
	require.Equal(t, http.StatusOK, code)
	code, _ = instance.do(t, http.MethodPost, "/api/auth/signout", upgraded.AccessToken, nil)
	require.Equal(t, http.StatusOK, code)
	code, _ = instance.do(t, http.MethodGet, "/api/auth/me", upgraded.AccessToken, nil)
	require.Equal(t, http.StatusUnauthorized, code)
}
