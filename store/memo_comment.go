package store

// MemoCommentAuthorizationSnapshot is the memo-local database state read by a
// driver before it creates an immutable COMMENT relation.
type MemoCommentAuthorizationSnapshot struct {
	ActorUserID int32
	Actor       MemoActorState

	ContextID         int32
	ContextCreatorID  int32
	ContextRowStatus  RowStatus
	ContextVisibility Visibility
}

// ValidateMemoCommentAuthorization validates participation in the context
// memo. The replying memo is authorized independently by the normal memo-create
// policy; no placement or audience is inherited from this snapshot.
func ValidateMemoCommentAuthorization(snapshot *MemoCommentAuthorizationSnapshot) error {
	return checkMemoCommentParticipation(snapshot, false)
}

// AuthorizeCommentContext checks the parent inside a memo creation
// transaction. Archive restoration can include an owned archived parent.
func (m *MemoMutation) AuthorizeCommentContext(snapshot *MemoCommentAuthorizationSnapshot) error {
	if snapshot == nil {
		return ErrMemoPermissionDenied
	}
	return checkMemoCommentParticipation(snapshot, m.RestoreArchivedComment)
}

func checkMemoCommentParticipation(snapshot *MemoCommentAuthorizationSnapshot, restoreArchivedComment bool) error {
	if snapshot == nil || snapshot.ActorUserID <= 0 || !snapshot.Actor.Active {
		return ErrMemoPermissionDenied
	}
	if snapshot.ContextID <= 0 || !isValidVisibility(snapshot.ContextVisibility) {
		return ErrMemoSpaceNotWritable
	}
	if snapshot.ContextRowStatus != Normal && !(restoreArchivedComment && snapshot.ContextRowStatus == Archived) {
		return ErrMemoSpaceNotWritable
	}
	if snapshot.ContextCreatorID != snapshot.ActorUserID {
		return ErrMemoPermissionDenied
	}
	return nil
}
