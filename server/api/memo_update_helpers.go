package api

import (
	"context"

	"github.com/pkg/errors"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/store"
)

func (s *APIService) loadUpdatedMemo(ctx context.Context, memoID int32) (*apipb.Memo, error) {
	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{ID: &memoID})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get memo")
	}
	if memo == nil {
		return nil, errors.New("memo not found")
	}

	reactions, err := s.Store.ListReactions(ctx, &store.FindReaction{
		MemoID: &memo.ID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list reactions")
	}
	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{
		MemoID: &memo.ID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list attachments")
	}
	relations, err := s.loadMemoRelations(ctx, memo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load memo relations")
	}
	memoMessage, err := s.convertMemoFromStore(ctx, memo, reactions, attachments, relations)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert memo")
	}

	return memoMessage, nil
}
