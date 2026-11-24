package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func TestNormalizePageSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pageSize int32
		want     int
	}{
		{
			name:     "default for zero",
			pageSize: 0,
			want:     DefaultPageSize,
		},
		{
			name:     "default for negative",
			pageSize: -1,
			want:     DefaultPageSize,
		},
		{
			name:     "preserves valid size",
			pageSize: 42,
			want:     42,
		},
		{
			name:     "clamps oversized size",
			pageSize: int32(MaxPageSize + 1),
			want:     MaxPageSize,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, normalizePageSize(tt.pageSize))
		})
	}
}

func createSpaceTestUser(ctx context.Context, t *testing.T, service *APIService, username string, role store.Role) *store.User {
	t.Helper()
	user, err := service.Store.CreateUser(ctx, &store.User{
		Username: username,
		Role:     role,
		Email:    username + "@example.com",
	})
	require.NoError(t, err)
	return user
}
