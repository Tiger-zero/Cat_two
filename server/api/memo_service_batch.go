package api

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/store"
)

// ListMCPMemoBatch uses the ordinary memo access scope for batched reads by
// known IDs, including archived notes. Results are formatted by the transport.
func (s *APIService) ListMCPMemoBatch(ctx context.Context, uids []string, maxTotalContentBytes int) ([]*store.Memo, error) {
	scope, user, err := s.resolveMemoAccessScope(ctx)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, status.Error(codes.Unauthenticated, "user not authenticated")
	}
	if len(uids) == 0 {
		return nil, nil
	}
	return s.Store.ListMemos(ctx, &store.FindMemo{
		UIDList:              uids,
		Access:               scope,
		Trash:                store.MemoTrashExclude,
		MaxTotalContentBytes: maxTotalContentBytes,
	})
}

// VisitOwnMemoTags visits tags from the active notes of the authenticated user.
func (s *APIService) VisitOwnMemoTags(ctx context.Context, visit func([]string)) error {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return err
	}
	if user == nil {
		return status.Error(codes.Unauthenticated, "user not authenticated")
	}
	return s.Store.VisitActiveMemoTags(ctx, user.ID, visit)
}
