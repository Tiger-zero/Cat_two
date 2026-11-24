package store

import (
	"context"

	"github.com/pkg/errors"
)

// ErrMemoPageChanged means a selected row changed between metadata and body reads.
var ErrMemoPageChanged = errors.New("memo page changed during read")

// MemoContentPage advances by Consumed rows in the ordered metadata query.
// A single oversize memo carries its UID and counts as one row for resuming.
type MemoContentPage struct {
	Memos        []*Memo
	Consumed     int
	HasMore      bool
	OversizedUID string
}

// ListMemoContentPage selects a prefix by size before loading Markdown. The
// second query enforces the budget again in case content changed meanwhile.
func (s *Store) ListMemoContentPage(ctx context.Context, find *FindMemo, limit, budget int) (MemoContentPage, error) {
	if limit <= 0 || budget <= 0 {
		return MemoContentPage{}, errors.New("memo page size and content budget must be positive")
	}
	metadata := *find
	withLookahead := limit + 1
	metadata.Limit = &withLookahead
	metadata.ExcludeContent = true
	metadata.IncludeContentSize = true
	rows, err := s.driver.ListMemos(ctx, &metadata)
	if err != nil || len(rows) == 0 {
		return MemoContentPage{}, err
	}
	if rows[0].ContentSizeBytes > int64(budget) {
		return MemoContentPage{Consumed: 1, HasMore: len(rows) > 1, OversizedUID: rows[0].UID}, nil
	}

	remaining := int64(budget)
	ids := make([]int32, 0, min(limit, len(rows)))
	for _, memo := range rows[:min(limit, len(rows))] {
		if memo.ContentSizeBytes > remaining {
			break
		}
		remaining -= memo.ContentSizeBytes
		ids = append(ids, memo.ID)
	}

	content := *find
	content.Limit, content.Offset = nil, nil
	content.IDList = ids
	content.MaxTotalContentBytes = budget
	memos, err := s.ListMemos(ctx, &content)
	if err != nil {
		return MemoContentPage{}, errors.Wrap(err, "read page Markdown")
	}
	if len(memos) != len(ids) {
		return MemoContentPage{}, ErrMemoPageChanged
	}
	// Keep the original order if an update moved one of the rows meanwhile.
	byID := make(map[int32]*Memo, len(memos))
	for _, memo := range memos {
		byID[memo.ID] = memo
	}
	ordered := make([]*Memo, 0, len(ids))
	for _, row := range rows[:len(ids)] {
		memo := byID[row.ID]
		if memo == nil || memo.Revision != row.Revision || memo.CreatedTs != row.CreatedTs || memo.UpdatedTs != row.UpdatedTs || memo.Pinned != row.Pinned {
			return MemoContentPage{}, ErrMemoPageChanged
		}
		ordered = append(ordered, memo)
	}
	return MemoContentPage{Memos: ordered, Consumed: len(ids), HasMore: len(rows) > len(ids)}, nil
}
