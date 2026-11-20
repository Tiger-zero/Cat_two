package api

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/store"
)

// GetUserUsage returns a user's usage. Cat_two applies no usage quota.
// Only the user or an admin may call it.
func (s *APIService) GetUserUsage(ctx context.Context, request *apipb.GetUserUsageRequest) (*apipb.UserUsage, error) {
	parent, ok := strings.CutSuffix(request.Name, "/usage")
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user usage name %q", request.Name)
	}
	currentUser, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user: %v", err)
	}
	if currentUser == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	username, err := parseUsernameFromName(parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user name: %v", err)
	}
	user, err := s.Store.GetUser(ctx, &store.FindUser{Username: &username})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}
	if user == nil {
		if currentUser.Role != store.RoleAdmin {
			return nil, status.Errorf(codes.PermissionDenied, "permission denied")
		}
		return nil, status.Errorf(codes.NotFound, "user not found")
	}
	if user.ID != currentUser.ID && currentUser.Role != store.RoleAdmin {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	memoCount, err := s.Store.CountUserMemos(ctx, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count memos: %v", err)
	}
	storageBytes, err := s.Store.GetAttachmentStorageUsage(ctx, user.ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to sum storage usage: %v", err)
	}
	return &apipb.UserUsage{
		Name:         BuildUserName(user.Username) + "/usage",
		MemoCount:    memoCount,
		StorageBytes: storageBytes,
		Limits:       &apipb.UserUsage_Limits{},
		LimitIndex:   -1,
	}, nil
}
