package store

// MemoCommentAuthorizationSnapshot is the memo-local database state read by a
// driver before it creates an immutable COMMENT relation.
type MemoCommentAuthorizationSnapshot struct {
	ActorUserID int32
	Actor       MemoActorState
	Personal    bool

	ContextID         int32
	ContextCreatorID  int32
	ContextRowStatus  RowStatus
	ContextVisibility Visibility
	ContextSpaceID    *int32

	ContextSpaceExists  bool
	ContextMemberActive bool
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
	participation := *snapshot
	participation.Personal = m.PersonalCreation
	return checkMemoCommentParticipation(&participation, m.RestoreArchivedComment)
}

func checkMemoCommentParticipation(snapshot *MemoCommentAuthorizationSnapshot, restoreArchivedComment bool) error {
	if snapshot == nil || snapshot.ActorUserID <= 0 || !snapshot.Actor.Active {
		return ErrMemoPermissionDenied
	}
	if snapshot.ContextID <= 0 || !isValidVisibility(snapshot.ContextVisibility) {
		return ErrMemoSpaceNotWritable
	}
	if snapshot.ContextRowStatus != Normal {
		if !restoreArchivedComment || snapshot.ContextRowStatus != Archived {
			return ErrMemoSpaceNotWritable
		}
		if snapshot.ContextCreatorID != snapshot.ActorUserID {
			return ErrMemoPermissionDenied
		}
	}
	if snapshot.Personal {
		if snapshot.ContextCreatorID != snapshot.ActorUserID {
			return ErrMemoPermissionDenied
		}
		return nil
	}
	if snapshot.ContextSpaceID == nil && snapshot.ContextVisibility == SpaceAudience {
		return ErrMemoSpaceNotWritable
	}
	if snapshot.ContextSpaceID != nil && !snapshot.ContextSpaceExists {
		return ErrMemoSpaceNotWritable
	}
	// An instance administrator participates in any structurally valid memo.
	if snapshot.Actor.Admin {
		return nil
	}
	// Participation in an assigned memo is narrower than reading it: active
	// membership is required even for PUBLIC and PROTECTED audiences.
	if snapshot.ContextSpaceID != nil && !snapshot.ContextMemberActive {
		return ErrMemoSpaceMembershipRequired
	}

	switch snapshot.ContextVisibility {
	case Private:
		if snapshot.ContextCreatorID != snapshot.ActorUserID {
			return ErrMemoPermissionDenied
		}
	case Protected, Public, SpaceAudience:
		// Any audience-specific placement and membership requirements were
		// validated above.
	default:
		return ErrMemoSpaceNotWritable
	}
	return nil
}
