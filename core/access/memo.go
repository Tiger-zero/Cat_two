// Package access defines transport-independent resource authorization policies
// shared by the server's API and HTTP adapters.
package access

import "github.com/usememos/memos/store"

// MemoReadDenial describes why a memo read was rejected.
type MemoReadDenial int

const (
	// MemoReadDenialNone means the read is allowed.
	MemoReadDenialNone MemoReadDenial = iota
	// MemoReadDenialNotFound hides missing, archived, and invalid memo state.
	MemoReadDenialNotFound
	// MemoReadDenialUnauthenticated means the resource requires a signed-in user.
	MemoReadDenialUnauthenticated
	// MemoReadDenialPermission means the signed-in user cannot read the resource.
	MemoReadDenialPermission
)

// MemoReadDecision is the outcome of evaluating memo read access.
type MemoReadDecision struct {
	Denial MemoReadDenial
}

// MemoReadContext contains the fully resolved authorization context for one
// memo. Relations never contribute authorization; callers evaluate each
// relation endpoint independently.
type MemoReadContext struct {
	Memo         *store.Memo
	Viewer       *store.User
	CreatorValid bool
	SpaceValid   bool
}

// Allowed reports whether the read is permitted.
func (d MemoReadDecision) Allowed() bool {
	return d.Denial == MemoReadDenialNone
}

// IsActiveUser reports whether the user exists and is in the normal lifecycle
// state, which every authenticated authorization decision requires.
func IsActiveUser(user *store.User) bool {
	return user != nil && user.RowStatus == store.Normal
}

// IsInstanceAdmin reports whether the user is an active application ADMIN.
func IsInstanceAdmin(user *store.User) bool {
	return IsActiveUser(user) && user.Role == store.RoleAdmin
}

// CanManageMemo reports whether the actor is the memo's active author.
func CanManageMemo(actor *store.User, memo *store.Memo) bool {
	return memo != nil && IsActiveUser(actor) && actor.ID == memo.CreatorID
}

// CanManageAttachment reports whether the actor is the attachment's active owner.
func CanManageAttachment(actor *store.User, attachment *store.Attachment) bool {
	return attachment != nil && IsActiveUser(actor) && actor.ID == attachment.CreatorID
}

// CheckMemoReadContext permits only the active creator. Historic audiences
// stay stored, but do not grant access to other people.
func CheckMemoReadContext(ctx MemoReadContext) MemoReadDecision {
	memo := ctx.Memo
	if memo == nil || !ctx.CreatorValid {
		return MemoReadDecision{Denial: MemoReadDenialNotFound}
	}
	if memo.Visibility != store.Public && memo.Visibility != store.Protected && memo.Visibility != store.Private && memo.Visibility != store.SpaceAudience {
		return MemoReadDecision{Denial: MemoReadDenialNotFound}
	}
	if memo.Visibility == store.SpaceAudience && (memo.SpaceID == nil || !ctx.SpaceValid) {
		return MemoReadDecision{Denial: MemoReadDenialNotFound}
	}
	if memo.RowStatus != store.Normal && memo.RowStatus != store.Archived {
		return MemoReadDecision{Denial: MemoReadDenialNotFound}
	}
	if memo.RowStatus == store.Archived && !CanManageMemo(ctx.Viewer, memo) {
		return MemoReadDecision{Denial: MemoReadDenialNotFound}
	}
	if !IsActiveUser(ctx.Viewer) {
		return MemoReadDecision{Denial: MemoReadDenialUnauthenticated}
	}
	if ctx.Viewer.ID != memo.CreatorID {
		return MemoReadDecision{Denial: MemoReadDenialPermission}
	}
	return MemoReadDecision{}
}
