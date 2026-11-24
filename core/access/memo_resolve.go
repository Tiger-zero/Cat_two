package access

import (
	"context"

	"github.com/usememos/memos/store"
)

// MemoReadStore is the store subset needed to resolve a memo's author.
type MemoReadStore interface {
	GetUser(ctx context.Context, find *store.FindUser) (*store.User, error)
}

// ResolveMemoReadContext resolves the persisted author and requesting user.
func ResolveMemoReadContext(ctx context.Context, s MemoReadStore, memo *store.Memo, viewer *store.User) (MemoReadContext, error) {
	read := MemoReadContext{Memo: memo, Viewer: viewer}
	if memo == nil {
		return read, nil
	}
	creator, err := s.GetUser(ctx, &store.FindUser{ID: &memo.CreatorID})
	if err != nil {
		return MemoReadContext{}, err
	}
	read.CreatorValid = creator != nil && creator.ID == memo.CreatorID &&
		(creator.RowStatus == store.Normal || creator.RowStatus == store.Archived)
	return read, nil
}
