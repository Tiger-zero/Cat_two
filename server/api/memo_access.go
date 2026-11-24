package api

import (
	"context"
	stderrors "errors"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/core/access"
	"github.com/usememos/memos/store"
)

// buildMemoReadContext resolves authorization inputs for exactly one memo.
// Relations never contribute access to either endpoint.
func (s *APIService) buildMemoReadContext(ctx context.Context, memo *store.Memo) (access.MemoReadContext, error) {
	viewer, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return access.MemoReadContext{}, status.Errorf(codes.Internal, "failed to get user")
	}
	return s.buildMemoReadContextForViewer(ctx, memo, viewer)
}

func (s *APIService) buildMemoReadContextForViewer(ctx context.Context, memo *store.Memo, viewer *store.User) (access.MemoReadContext, error) {
	if memo == nil {
		return access.MemoReadContext{}, status.Error(codes.NotFound, "memo not found")
	}
	readContext, err := access.ResolveMemoReadContext(ctx, s.Store, memo, viewer)
	if err != nil {
		return access.MemoReadContext{}, status.Errorf(codes.Internal, "failed to resolve memo access")
	}
	return readContext, nil
}

func (s *APIService) checkMemoReadAccess(ctx context.Context, memo *store.Memo) error {
	readContext, err := s.buildMemoReadContext(ctx, memo)
	if err != nil {
		return err
	}
	return memoAccessDecisionError(access.CheckMemoReadContext(readContext))
}

func memoAccessDecisionError(decision access.MemoReadDecision) error {
	switch decision.Denial {
	case access.MemoReadDenialNone:
		return nil
	case access.MemoReadDenialNotFound:
		return status.Error(codes.NotFound, "memo not found")
	case access.MemoReadDenialUnauthenticated:
		return status.Error(codes.Unauthenticated, "user not authenticated")
	default:
		return status.Error(codes.PermissionDenied, "permission denied")
	}
}

// newMemoAccessScope returns the memo-local authorization predicate for a
// caller. Drivers apply it as a database predicate before LIMIT/OFFSET so
// inaccessible rows can neither leak nor skew counts and pagination.
func newMemoAccessScope(currentUser *store.User) *store.MemoAccessScope {
	accessScope := &store.MemoAccessScope{}
	if currentUser != nil {
		accessScope.UserID = &currentUser.ID
	}
	return accessScope
}

// resolveMemoAccessScope resolves the caller and builds their memo access
// scope. An anonymous scope matches no rows.
func (s *APIService) resolveMemoAccessScope(ctx context.Context) (*store.MemoAccessScope, *store.User, error) {
	currentUser, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to get current user")
	}
	return newMemoAccessScope(currentUser), currentUser, nil
}

func mapMemoWriteError(err error, operation string) error {
	switch {
	case stderrors.Is(err, store.ErrMemoRevisionConflict):
		return status.Error(codes.Aborted, "memo revision changed")
	case stderrors.Is(err, store.ErrMemoMutationConflict):
		return status.Errorf(codes.FailedPrecondition, "memo state changed: %v", err)
	case stderrors.Is(err, store.ErrMemoSpaceNotWritable):
		return status.Error(codes.FailedPrecondition, "memo space is no longer writable")
	case stderrors.Is(err, store.ErrMemoPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case stderrors.Is(err, store.ErrMemoShareConflict):
		return status.Error(codes.FailedPrecondition, "revoke active shares before using the SPACE audience")
	default:
		return status.Errorf(codes.Internal, "%s: %v", operation, err)
	}
}
