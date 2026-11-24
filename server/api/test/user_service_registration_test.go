package test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/store"
)

func TestCreateUserRegistration(t *testing.T) {
	ctx := context.Background()

	t.Run("only the first administrator can be created or validated", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		request := &apipb.CreateUserRequest{
			User:         &apipb.User{Username: "owner", Email: "owner@example.com", Password: "password123", Role: apipb.User_USER},
			ValidateOnly: true,
		}
		preview, err := ts.Service.CreateUser(ctx, request)
		require.NoError(t, err)
		require.Equal(t, apipb.User_ADMIN, preview.Role)
		users, err := ts.Store.ListUsers(ctx, &store.FindUser{})
		require.NoError(t, err)
		require.Empty(t, users, "validation does not claim the instance")

		request.ValidateOnly = false
		created, err := ts.Service.CreateUser(ctx, request)
		require.NoError(t, err)
		require.Equal(t, "users/owner", created.Name)
		require.Equal(t, apipb.User_ADMIN, created.Role)
		owner, err := ts.Store.GetUser(ctx, &store.FindUser{Username: &request.User.Username})
		require.NoError(t, err)
		require.NotNil(t, owner)
		require.Equal(t, store.RoleAdmin, owner.Role)

		for _, caller := range []context.Context{ctx, ts.CreateUserContext(ctx, owner.ID)} {
			for _, validateOnly := range []bool{false, true} {
				_, err := ts.Service.CreateUser(caller, &apipb.CreateUserRequest{
					User:         &apipb.User{Username: "other", Password: "password123", Role: apipb.User_ADMIN},
					ValidateOnly: validateOnly,
				})
				require.Equal(t, codes.PermissionDenied, status.Code(err))
			}
		}
		users, err = ts.Store.ListUsers(ctx, &store.FindUser{})
		require.NoError(t, err)
		require.Len(t, users, 1)
	})

	t.Run("CreateUser rejects empty password", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		_, err := ts.Service.CreateUser(ctx, &apipb.CreateUserRequest{
			User: &apipb.User{
				Username: "newuser",
				Email:    "newuser@example.com",
				Password: "",
			},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "password must not be empty")
	})

	t.Run("CreateUser rejects invalid writable usernames", func(t *testing.T) {
		for _, username := range []string{"alice@example.com", "legacy_user"} {
			t.Run(username, func(t *testing.T) {
				ts := NewTestService(t)
				defer ts.Cleanup()

				_, err := ts.Service.CreateUser(ctx, &apipb.CreateUserRequest{
					User: &apipb.User{
						Username: username,
						Email:    "newuser@example.com",
						Password: "password123",
					},
				})
				require.Error(t, err)
				require.Contains(t, err.Error(), "invalid username")
			})
		}
	})

	t.Run("CreateUser validate only rejects invalid writable username", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		_, err := ts.Service.CreateUser(ctx, &apipb.CreateUserRequest{
			User: &apipb.User{
				Username: "alice@example.com",
				Email:    "newuser@example.com",
				Password: "password123",
			},
			ValidateOnly: true,
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid username")
	})

	t.Run("CreateUser requires user_id to match username", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		_, err := ts.Service.CreateUser(ctx, &apipb.CreateUserRequest{
			User: &apipb.User{
				Username: "alice",
				Email:    "alice@example.com",
				Password: "password123",
			},
			UserId: "bob",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "user_id must match user.username")
	})

	t.Run("UpdateUser rejects empty password", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "alice")
		require.NoError(t, err)

		authCtx := ts.CreateUserContext(ctx, user.ID)
		_, err = ts.Service.UpdateUser(authCtx, &apipb.UpdateUserRequest{
			User: &apipb.User{
				Name:     api.BuildUserName(user.Username),
				Password: "",
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"password"}},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "password must not be empty")
	})

	t.Run("UpdateUser rejects missing user message", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "missing-message")
		require.NoError(t, err)

		authCtx := ts.CreateUserContext(ctx, user.ID)
		_, err = ts.Service.UpdateUser(authCtx, &apipb.UpdateUserRequest{
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"display_name"}},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "user is required")
	})

	t.Run("CreateUser concurrent first setup creates one admin", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		const workers = 12
		var wg sync.WaitGroup
		for i := range workers {
			wg.Go(func() {
				_, _ = ts.Service.CreateUser(ctx, &apipb.CreateUserRequest{
					User: &apipb.User{
						Username: fmt.Sprintf("setup-user-%d", i),
						Email:    "setup-user@example.com",
						Password: "password123",
					},
				})
			})
		}
		wg.Wait()

		users, err := ts.Store.ListUsers(ctx, &store.FindUser{})
		require.NoError(t, err)
		require.Len(t, users, 1)
		require.Equal(t, store.RoleAdmin, users[0].Role)
	})

	t.Run("UpdateUser state requires admin", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "state-user")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		_, err = ts.Service.UpdateUser(userCtx, &apipb.UpdateUserRequest{
			User: &apipb.User{
				Name:  api.BuildUserName(user.Username),
				State: apipb.State_ARCHIVED,
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "permission denied")

		admin, err := ts.CreateHostUser(ctx, "state-admin")
		require.NoError(t, err)
		adminCtx := ts.CreateUserContext(ctx, admin.ID)
		updated, err := ts.Service.UpdateUser(adminCtx, &apipb.UpdateUserRequest{
			User: &apipb.User{
				Name:  api.BuildUserName(user.Username),
				State: apipb.State_ARCHIVED,
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
		})
		require.NoError(t, err)
		require.Equal(t, apipb.State_ARCHIVED, updated.State)
	})

	t.Run("archived user context is rejected", func(t *testing.T) {
		ts := NewTestService(t)
		defer ts.Cleanup()

		user, err := ts.CreateRegularUser(ctx, "archived-access-user")
		require.NoError(t, err)
		userCtx := ts.CreateUserContext(ctx, user.ID)
		archived := store.Archived
		_, err = ts.Store.UpdateUser(ctx, &store.UpdateUser{
			ID:        user.ID,
			RowStatus: &archived,
		})
		require.NoError(t, err)

		_, err = ts.Service.GetCurrentUser(userCtx, &apipb.GetCurrentUserRequest{})
		require.Error(t, err)

		_, err = ts.Service.CreateMemo(userCtx, &apipb.CreateMemoRequest{
			Memo: &apipb.Memo{
				Content: "should not be created",
			},
		})
		require.Error(t, err)

		_, err = ts.Service.UpdateUser(userCtx, &apipb.UpdateUserRequest{
			User: &apipb.User{
				Name:  api.BuildUserName(user.Username),
				State: apipb.State_NORMAL,
			},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
		})
		require.Error(t, err)
	})
}
