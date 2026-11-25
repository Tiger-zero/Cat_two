package store

// MemoCommentAuthorizationSnapshot is the memo-local database state read by a
// driver before it creates an immutable COMMENT relation.
type MemoCommentAuthorizationSnapshot struct {
	ActorUserID int32
	Actor       MemoActorState

	ContextID         int32
	ContextCreatorID  int32
	ContextVisibility Visibility
}

// ValidateMemoCommentAuthorization validates participation in the context
// memo. The replying memo is authorized independently by the normal memo-create
// policy; no placement or audience is inherited from this snapshot.
func ValidateMemoCommentAuthorization(snapshot *MemoCommentAuthorizationSnapshot) error {
	if snapshot == nil || snapshot.ActorUserID <= 0 || !snapshot.Actor.Active {
		return ErrMemoPermissionDenied
	}
	if snapshot.ContextID <= 0 || !isValidVisibility(snapshot.ContextVisibility) {
		return ErrMemoContextUnavailable
	}
	if snapshot.ContextCreatorID != snapshot.ActorUserID {
		return ErrMemoPermissionDenied
	}
	return nil
}
