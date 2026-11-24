package store

import (
	"context"
	"encoding/json"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/usememos/memos/filter"
	"github.com/usememos/memos/internal/identifier"
	storepb "github.com/usememos/memos/proto/gen/store"
)

// ErrMemoContextUnavailable indicates an absent or inactive comment target.
var ErrMemoContextUnavailable = errors.New("memo context is unavailable")

// ErrMemoPermissionDenied indicates that the actor no longer owns the memo
// being mutated.
var ErrMemoPermissionDenied = errors.New("memo mutation permission denied")

// ErrMemoContentBudgetExceeded means the requested Markdown exceeds the read budget.
var ErrMemoContentBudgetExceeded = errors.New("memo content budget exceeded")

// Visibility is the type of a visibility.
type Visibility string

const (
	// Public is the PUBLIC visibility.
	Public Visibility = "PUBLIC"
	// Protected is the PROTECTED visibility.
	Protected Visibility = "PROTECTED"
	// Private is the PRIVATE visibility.
	Private Visibility = "PRIVATE"
)

func (v Visibility) String() string {
	return string(v)
}

// MemoCreationChannelUnknown represents notes without recorded provenance.
const MemoCreationChannelUnknown = "unknown"

// IsMemoCreationChannel validates the stable identifiers accepted from clients and archives.
func IsMemoCreationChannel(value string) bool {
	switch value {
	case MemoCreationChannelUnknown, "web", "telegram", "browser_extension", "wechat":
		return true
	default:
		return false
	}
}

type Memo struct {
	// ID is the system generated unique identifier for the memo.
	ID int32
	// UID is the user defined unique identifier for the memo.
	UID string

	// Standard fields
	RowStatus RowStatus
	CreatorID int32
	CreatedTs int64
	UpdatedTs int64

	// Domain specific fields
	Content    string
	BodyDigest string
	Visibility Visibility
	Pinned     bool
	Payload    *storepb.MemoPayload
	DeletedTs  *int64
	// Revision changes whenever note content, metadata, or attachments are edited.
	Revision int64
	// CreationRequestDigest identifies the original idempotent create request.
	CreationRequestDigest string

	// Composed fields
	ParentUID        *string
	ContentSizeBytes int64
}

type FindMemo struct {
	ID  *int32
	UID *string

	IDList  []int32
	UIDList []string

	// Standard fields
	RowStatus *RowStatus
	CreatorID *int32

	// Domain specific fields
	VisibilityList       []Visibility
	CommentContextMemoID *int32
	Access               *MemoAccessScope
	ExcludeContent       bool
	IncludeContentSize   bool
	// MaxTotalContentBytes bounds the total Markdown bytes returned. Zero leaves it unbounded.
	MaxTotalContentBytes int
	ExcludeComments      bool
	Filters              []string
	PreparedFilter       *filter.Program
	Trash                MemoTrashScope
	DeletedAfterTs       *int64
	DeletedThroughTs     *int64

	// Pagination
	Limit  *int
	Offset *int

	// Ordering
	OrderByPinned    bool
	OrderByUpdatedTs bool
	OrderByTimeAsc   bool
}

type FindMemoPayload struct {
	Raw                *string
	TagSearch          []string
	HasLink            bool
	HasTaskList        bool
	HasCode            bool
	HasIncompleteTasks bool
}

// RefreshMemoDerivedPayload replaces the content-derived part of a memo's
// payload, its tags and property, computed from Content.
type RefreshMemoDerivedPayload struct {
	ID int32
	// Content is the memo content the tags and property were computed from.
	// The refresh applies only while the stored content still equals it.
	Content  string
	Tags     []string
	Property *storepb.MemoPayload_Property
}

// JSON returns the tags and property encoded the way memo payloads are stored.
func (r *RefreshMemoDerivedPayload) JSON() (tags string, property string, err error) {
	tagList := r.Tags
	if tagList == nil {
		tagList = []string{}
	}
	tagBytes, err := json.Marshal(tagList)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to marshal memo tags")
	}
	prop := r.Property
	if prop == nil {
		prop = &storepb.MemoPayload_Property{}
	}
	propertyBytes, err := protojson.Marshal(prop)
	if err != nil {
		return "", "", errors.Wrap(err, "failed to marshal memo property")
	}
	return string(tagBytes), string(propertyBytes), nil
}

type UpdateMemo struct {
	ID         int32
	UID        *string
	CreatedTs  *int64
	UpdatedTs  *int64
	RowStatus  *RowStatus
	Content    *string
	BodyDigest string
	Visibility *Visibility
	Pinned     *bool
	Payload    *storepb.MemoPayload
	// Policy is set by transport-facing author mutations. Drivers revalidate it
	// in the same transaction as the update; nil preserves trusted internal and
	// migration callers.
	Policy *MemoWritePolicy
	// ExpectedRevision optionally requires the revision read by the client.
	ExpectedRevision *int64
}

// MemoWritePolicy identifies the actor of a transport mutation. The driver
// validates ownership against the memo state inside the write transaction.
type MemoWritePolicy struct {
	ActorUserID int32
	// CreatingShare applies the legacy storage invariant to old share data.
	CreatingShare bool
}

// MemoActorState is the actor's user row state as read inside a mutation
// transaction. Admin marks an active instance administrator; see
// access.IsInstanceAdmin for what that grants. A missing user is the zero
// value.
type MemoActorState struct {
	Active bool
	Admin  bool
}

// NewMemoActorState derives the actor state from a user row. An archived
// administrator is neither active nor privileged.
func NewMemoActorState(rowStatus RowStatus, role Role) MemoActorState {
	active := rowStatus == Normal
	return MemoActorState{Active: active, Admin: active && role == RoleAdmin}
}

// MemoWriteSnapshot is the current database state used to validate a
// MemoWritePolicy. Driver packages populate it in the mutation transaction.
type MemoWriteSnapshot struct {
	ActorIsAdmin bool
	CreatorID    int32
	RowStatus    RowStatus
	Visibility   Visibility
}

// MemoAccessScope authorizes the memo author before pagination. A scope with
// no user denies all reads. Trusted store scans can omit the scope.
type MemoAccessScope struct {
	UserID *int32
}

type DeleteMemo struct {
	ID int32
}

func (s *Store) CreateMemo(ctx context.Context, create *Memo) (*Memo, error) {
	if err := validateMemoCreate(create); err != nil {
		return nil, err
	}
	digest, err := s.stageMemoBody(ctx, create.Content)
	if err != nil {
		return nil, err
	}
	create.BodyDigest = digest
	return s.driver.CreateMemo(ctx, create)
}

// CreateMemoComment atomically creates one independent memo and its immutable
// COMMENT relation to a context memo.
func (s *Store) CreateMemoComment(ctx context.Context, create *Memo, contextMemoID, actorUserID int32) (*Memo, error) {
	if create == nil || contextMemoID <= 0 || actorUserID <= 0 || create.CreatorID != actorUserID {
		return nil, errors.New("comment creation requires memo, context, and matching actor")
	}
	if create.Visibility == "" {
		create.Visibility = Private
	}
	if err := s.ApplyMemoMutation(ctx, &MemoMutation{
		MemoCreate:           create,
		CommentContextMemoID: &contextMemoID,
		MemoCreatorID:        actorUserID,
		ExpectedMemoContent:  create.Content,
	}); err != nil {
		return nil, err
	}
	return create, nil
}

func validateMemoCreate(create *Memo) error {
	if create == nil {
		return errors.New("memo is required")
	}
	if !identifier.UIDMatcher.MatchString(create.UID) {
		return errors.New("invalid uid")
	}
	if !isValidVisibility(create.Visibility) {
		return errors.New("invalid visibility")
	}
	return nil
}

func (s *Store) ListMemos(ctx context.Context, find *FindMemo) ([]*Memo, error) {
	memos, err := s.driver.ListMemos(ctx, find)
	if err != nil || find.ExcludeContent {
		return memos, err
	}
	if err := s.hydrateMemoBodies(ctx, memos, find.MaxTotalContentBytes); err != nil {
		return nil, err
	}
	return memos, nil
}

// VisitActiveMemoTags visits the tags of each active top-level memo owned by the active account.
func (s *Store) VisitActiveMemoTags(ctx context.Context, creatorID int32, visit func([]string)) error {
	rows, err := s.driver.QueryActiveMemoTags(ctx, creatorID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return err
		}
		if len(encoded) == 0 {
			continue
		}
		var tags []string
		if err := json.Unmarshal(encoded, &tags); err != nil {
			return errors.Wrap(err, "decode memo tags")
		}
		if len(tags) > 0 {
			visit(tags)
		}
	}
	return rows.Err()
}

// CountUserMemos counts the memos a user created, comments included, in every
// row status.
func (s *Store) CountUserMemos(ctx context.Context, creatorID int32) (int64, error) {
	return s.driver.CountUserMemos(ctx, creatorID)
}

func (s *Store) GetMemo(ctx context.Context, find *FindMemo) (*Memo, error) {
	list, err := s.ListMemos(ctx, find)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}

	memo := list[0]
	return memo, nil
}

func (s *Store) UpdateMemo(ctx context.Context, update *UpdateMemo) error {
	if update.UID != nil && !identifier.UIDMatcher.MatchString(*update.UID) {
		return errors.New("invalid uid")
	}
	if update.Visibility != nil && !isValidVisibility(*update.Visibility) {
		return errors.New("invalid visibility")
	}
	if err := validateMemoWritePolicy(update.Policy); err != nil {
		return err
	}
	if update.Content != nil {
		digest, err := s.stageMemoBody(ctx, *update.Content)
		if err != nil {
			return err
		}
		update.BodyDigest = digest
	}
	return s.driver.UpdateMemo(ctx, update)
}

// RefreshMemoDerivedPayload rewrites a memo's payload tags and property in
// place, and only if the memo's content has not changed since it was read. It
// leaves every other payload field, such as the location, and updated_ts as
// they are, so a background rebuild can never undo a concurrent edit. A memo
// that was edited or deleted in the meantime is left alone.
func (s *Store) RefreshMemoDerivedPayload(ctx context.Context, refresh *RefreshMemoDerivedPayload) error {
	return s.driver.RefreshMemoDerivedPayload(ctx, refresh)
}

func validateMemoWritePolicy(policy *MemoWritePolicy) error {
	if policy == nil {
		return nil
	}
	if policy.ActorUserID <= 0 {
		return errors.New("memo write policy requires actor")
	}
	return nil
}

// ValidateMemoWriteSnapshot applies the transport-independent write
// invariants to current database state.
func ValidateMemoWriteSnapshot(policy *MemoWritePolicy, update *UpdateMemo, snapshot *MemoWriteSnapshot) error {
	if err := validateMemoWritePolicy(policy); err != nil {
		return err
	}
	if policy == nil || snapshot == nil {
		return errors.New("memo write policy snapshot is required")
	}
	if snapshot.CreatorID != policy.ActorUserID {
		return ErrMemoPermissionDenied
	}
	if (snapshot.RowStatus != Normal && snapshot.RowStatus != Archived) || !isValidVisibility(snapshot.Visibility) {
		return ErrMemoMutationConflict
	}
	if policy.CreatingShare && snapshot.RowStatus != Normal {
		return ErrMemoMutationConflict
	}
	if update != nil && update.Visibility != nil && *update.Visibility != Private {
		return ErrMemoPermissionDenied
	}
	return nil
}

func isValidVisibility(visibility Visibility) bool {
	return visibility == Public || visibility == Protected || visibility == Private
}

func (s *Store) DeleteMemo(ctx context.Context, delete *DeleteMemo) error {
	if delete == nil || delete.ID <= 0 {
		return errors.New("memo deletion requires a memo")
	}
	memo, err := s.GetMemo(ctx, &FindMemo{ID: &delete.ID})
	if err != nil {
		return err
	}
	if memo == nil {
		return nil
	}
	result, err := s.DeleteMemoWithPolicy(ctx, &DeleteMemoWithPolicy{MemoID: memo.ID, ActorUserID: memo.CreatorID})
	if err != nil {
		return err
	}
	return s.deleteAttachmentStorageSnapshots(ctx, result.Attachments)
}
