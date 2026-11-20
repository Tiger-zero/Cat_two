package api

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/usememos/memos/core/access"
	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/store"
)

// CreateMemoShare is disabled because notes are private to their authors.
func (s *APIService) CreateMemoShare(context.Context, *apipb.CreateMemoShareRequest) (*apipb.MemoShare, error) {
	return nil, status.Error(codes.FailedPrecondition, "memo sharing is disabled")
}

// ListMemoShares lists all share links for a memo.
// Only the memo's creator may call this.
func (s *APIService) ListMemoShares(ctx context.Context, request *apipb.ListMemoSharesRequest) (*apipb.ListMemoSharesResponse, error) {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}

	memoUID, err := ExtractMemoUIDFromName(request.Parent)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid memo name: %v", err)
	}
	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{UID: &memoUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get memo")
	}
	if memo == nil {
		return nil, status.Errorf(codes.NotFound, "memo not found")
	}
	if !access.CanManageMemo(user, memo) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	if err := s.requireAssignedMemoWritable(ctx, memo, user); err != nil {
		return nil, err
	}

	shares, err := s.Store.ListMemoShares(ctx, &store.FindMemoShare{MemoID: &memo.ID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list memo shares")
	}

	response := &apipb.ListMemoSharesResponse{}
	for _, ms := range shares {
		response.MemoShares = append(response.MemoShares, convertMemoShareFromStore(ms, memo.UID))
	}
	return response, nil
}

// DeleteMemoShare revokes a share link.
// Only the memo's creator may call this.
func (s *APIService) DeleteMemoShare(ctx context.Context, request *apipb.DeleteMemoShareRequest) (*emptypb.Empty, error) {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}

	// name format: memos/{memoUID}/shares/{shareToken}
	tokens, err := GetNameParentTokens(request.Name, MemoNamePrefix, MemoShareNamePrefix)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid share name: %v", err)
	}
	memoUID, shareToken := tokens[0], tokens[1]

	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{UID: &memoUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get memo")
	}
	if memo == nil {
		return nil, status.Errorf(codes.NotFound, "memo not found")
	}
	if !access.CanManageMemo(user, memo) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	ms, err := s.Store.GetMemoShare(ctx, &store.FindMemoShare{UID: &shareToken})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get memo share")
	}
	if ms == nil || ms.MemoID != memo.ID {
		return nil, status.Errorf(codes.NotFound, "memo share not found")
	}

	if err := s.Store.DeleteMemoShare(ctx, &store.DeleteMemoShare{
		UID:    &shareToken,
		MemoID: &memo.ID,
		Policy: memoWritePolicy(user.ID, false),
	}); err != nil {
		return nil, mapMemoWriteError(err, "failed to delete memo share")
	}
	return &emptypb.Empty{}, nil
}

// GetSharedMemo no longer grants access through a legacy share token.
func (s *APIService) GetSharedMemo(context.Context, *apipb.GetSharedMemoRequest) (*apipb.Memo, error) {
	return nil, status.Error(codes.NotFound, "not found")
}

// convertMemoShareFromStore converts a store MemoShare to the proto MemoShare message.
// name format: memos/{memoUID}/shares/{shareToken}.
func convertMemoShareFromStore(ms *store.MemoShare, memoUID string) *apipb.MemoShare {
	name := fmt.Sprintf("%s%s/%s%s", MemoNamePrefix, memoUID, MemoShareNamePrefix, ms.UID)
	pb := &apipb.MemoShare{
		Name:       name,
		CreateTime: timestamppb.New(time.Unix(ms.CreatedTs, 0)),
	}
	if ms.ExpiresTs != nil {
		pb.ExpireTime = timestamppb.New(time.Unix(*ms.ExpiresTs, 0))
	}
	return pb
}
