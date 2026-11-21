package store

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// MemoTrashRetention is the time an owner has to restore a deleted memo.
const MemoTrashRetention = 30 * 24 * time.Hour

// MemoTrashScope controls whether a memo query includes recycle-bin entries.
type MemoTrashScope int

const (
	// MemoTrashExclude is the default for ordinary memo queries.
	MemoTrashExclude MemoTrashScope = iota
	// MemoTrashOnly selects only the individually deleted roots in the recycle bin.
	MemoTrashOnly
	// MemoTrashInclude also includes deleted rows that still reserve their UIDs.
	MemoTrashInclude
)

// MemoTrashAction identifies a recycle-bin state transition.
type MemoTrashAction int

const (
	// MemoTrashMove hides the memo and its replies without deleting their data.
	MemoTrashMove MemoTrashAction = iota + 1
	// MemoTrashRestore brings back the memo and the replies deleted with it.
	MemoTrashRestore
	// MemoTrashPurge permanently deletes the memo and its reply subtree.
	MemoTrashPurge
)

// MemoTrashMutation identifies a memo lifecycle operation. An actor ID of
// zero permits only server-initiated expiry, never an early purge.
type MemoTrashMutation struct {
	MemoID      int32
	ActorUserID int32
	Action      MemoTrashAction
	NowTs       int64
}

// Cutoff returns the last deletion timestamp that is no longer restorable.
func (m *MemoTrashMutation) Cutoff() int64 {
	return m.NowTs - int64(MemoTrashRetention/time.Second)
}

// ValidateMemoTrashState checks the locked memo row before a transition.
func ValidateMemoTrashState(m *MemoTrashMutation, creatorID int32, deletedTs *int64, isTrashRoot bool) error {
	if m.ActorUserID != 0 && m.ActorUserID != creatorID {
		return ErrMemoPermissionDenied
	}
	if m.Action == MemoTrashMove {
		if deletedTs != nil {
			return ErrMemoMutationConflict
		}
		return nil
	}
	if deletedTs == nil || !isTrashRoot {
		return ErrMemoMutationConflict
	}
	expired := *deletedTs <= m.Cutoff()
	if (m.Action == MemoTrashRestore && expired) || (m.ActorUserID == 0 && !expired) {
		return ErrMemoMutationConflict
	}
	return nil
}

// ApplyMemoTrash atomically changes a memo's recycle-bin state and returns any
// attachments removed by permanent deletion for external storage cleanup.
func (s *Store) ApplyMemoTrash(ctx context.Context, mutation *MemoTrashMutation) ([]*Attachment, error) {
	if mutation == nil || mutation.MemoID <= 0 || mutation.NowTs <= 0 || mutation.ActorUserID < 0 {
		return nil, errors.New("invalid memo recycle-bin mutation")
	}
	switch mutation.Action {
	case MemoTrashMove, MemoTrashRestore:
		if mutation.ActorUserID == 0 {
			return nil, errors.New("memo recycle-bin mutation requires an actor")
		}
	case MemoTrashPurge:
		// The maintenance job may permanently delete expired entries.
	default:
		return nil, errors.New("invalid memo recycle-bin action")
	}
	return s.driver.ApplyMemoTrash(ctx, mutation)
}
