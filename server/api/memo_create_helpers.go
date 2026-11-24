package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/usememos/memos/core/memopayload"
	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

const mcpConnectionTag = "MCP连接"

type mcpMemoCreationContextKey struct{}

// WithMCPMemoCreation marks a request dispatched by the in-process MCP tool handler.
func WithMCPMemoCreation(ctx context.Context) context.Context {
	return context.WithValue(ctx, mcpMemoCreationContextKey{}, true)
}

func mapMemoCreateError(err error, uid, operation string) error {
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return memoAlreadyExists(uid)
	}
	return mapMemoWriteError(err, operation)
}

func memoAlreadyExists(uid string) error {
	return status.Errorf(codes.AlreadyExists, "memo with ID %q already exists", uid)
}

// memoCreationDigest fingerprints the original request independently of the
// Markdown and attachment state later used to prepare the write. A replay
// still matches after that state changes, such as an attachment being bound.
func memoCreationDigest(uid string, idempotent bool, request proto.Message) (string, error) {
	if !idempotent {
		return "", nil
	}
	if strings.TrimSpace(uid) == "" {
		return "", status.Error(codes.InvalidArgument, "idempotent creation requires an explicit ID")
	}
	encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(request)
	if err != nil {
		return "", status.Error(codes.InvalidArgument, "invalid memo creation request")
	}
	// Include the RPC request type, so a comment and a top-level memo cannot
	// claim the same operation even if their wire representations overlap.
	name := request.ProtoReflect().Descriptor().FullName()
	digest := sha256.New()
	_, _ = digest.Write([]byte(string(name) + "\x00"))
	_, _ = digest.Write(encoded)
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func (s *APIService) replayMemoCreation(ctx context.Context, userID int32, uid, digest string) (*apipb.Memo, error) {
	if digest == "" {
		return nil, nil
	}
	existing, err := s.Store.GetMemo(ctx, &store.FindMemo{UID: &uid, ExcludeContent: true, Trash: store.MemoTrashInclude})
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to look up memo creation result")
	}
	if existing == nil {
		return nil, nil
	}
	if existing.CreatorID != userID || existing.CreationRequestDigest != digest || existing.DeletedTs != nil {
		return nil, memoAlreadyExists(uid)
	}
	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{ID: &existing.ID})
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load memo creation result")
	}
	if memo == nil {
		return nil, status.Error(codes.NotFound, "memo not found")
	}
	return s.readMemoResponse(ctx, memo)
}

// saveNewMemo either commits the memo with its initial attachments and
// relations, or returns the resource produced by an exact prior request.
func (s *APIService) saveNewMemo(ctx context.Context, user *store.User, input *apipb.Memo, uid, digest string, parent *store.Memo) (*store.Memo, *apipb.Memo, error) {
	if replay, err := s.replayMemoCreation(ctx, user.ID, uid, digest); replay != nil || err != nil {
		return nil, replay, err
	}
	var parentID *int32
	if parent != nil {
		if parent.RowStatus != store.Normal {
			return nil, nil, status.Error(codes.FailedPrecondition, "cannot comment on an archived memo")
		}
		parentID = &parent.ID
	}
	addConnectionTag := parent == nil && (auth.HasPersonalAccessToken(ctx) || ctx.Value(mcpMemoCreationContextKey{}) != nil)
	prepared, err := s.prepareMemoCreate(ctx, user, input, uid, addConnectionTag)
	if err == nil {
		prepared.memo.CreationRequestDigest = digest
		err = s.createMemoWithMutation(ctx, user, prepared.memo, parentID, prepared.attachments, prepared.requiredAttachmentIDs, prepared.referenceRelations, false)
		if err == nil {
			s.SSEHub.publishMemoChanged()
			return prepared.memo, nil, nil
		}
		err = mapMemoCreateError(err, uid, "failed to create memo")
	}
	// A concurrent request may have committed since the first lookup. It
	// could have claimed the same attachments before we even got to insert.
	if replay, replayErr := s.replayMemoCreation(ctx, user.ID, uid, digest); replay != nil || replayErr != nil {
		return nil, replay, replayErr
	}
	return nil, nil, err
}

type preparedMemoCreate struct {
	memo                  *store.Memo
	attachments           *preparedMemoAttachments
	requiredAttachmentIDs []int32
	referenceRelations    []*store.MemoRelation
}

// prepareMemoCreate validates and prepares the fields shared by top-level
// memo and comment creation. It does not write database or external state.
func (s *APIService) prepareMemoCreate(ctx context.Context, user *store.User, input *apipb.Memo, uid string, addConnectionTag bool) (*preparedMemoCreate, error) {
	if user == nil || input == nil {
		return nil, status.Error(codes.InvalidArgument, "memo is required")
	}
	channel := input.GetCreationChannel()
	if channel != "" && !store.IsMemoCreationChannel(channel) {
		return nil, status.Error(codes.InvalidArgument, "invalid memo creation_channel")
	}
	visibility, err := validateCreateMemoVisibility(input.Visibility)
	if err != nil {
		return nil, err
	}
	memo := &store.Memo{
		UID:        uid,
		CreatorID:  user.ID,
		Content:    input.Content,
		Visibility: visibility,
	}
	if input.GetSpace() != "" {
		return nil, retiredModuleError()
	}
	if input.CreateTime != nil && input.CreateTime.IsValid() {
		memo.CreatedTs = input.CreateTime.AsTime().Unix()
	}
	if input.UpdateTime != nil && input.UpdateTime.IsValid() {
		memo.UpdatedTs = input.UpdateTime.AsTime().Unix()
	}
	if addConnectionTag {
		memo.Content, err = s.MarkdownService.EnsureTag(memo.Content, mcpConnectionTag)
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to add memo connection tag")
		}
	}

	if err := memopayload.RebuildMemoPayload(ctx, memo, s.MarkdownService); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to rebuild memo payload: %v", err)
	}
	memo.Payload.CreationChannel = channel

	attachments, err := s.prepareMemoAttachments(ctx, user, memo, input.Attachments)
	if err != nil {
		return nil, err
	}
	requiredAttachmentIDs, err := s.resolveMemoAttachmentReferences(memo.Content, attachments.normalized)
	if err != nil {
		return nil, err
	}
	relations, err := s.prepareMemoRelations(ctx, memo, input.Relations)
	if err != nil {
		return nil, err
	}
	return &preparedMemoCreate{
		memo:                  memo,
		attachments:           attachments,
		requiredAttachmentIDs: requiredAttachmentIDs,
		referenceRelations:    relations,
	}, nil
}
