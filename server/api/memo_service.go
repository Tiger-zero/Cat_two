package api

import (
	"context"
	stderrors "errors"
	"log/slog"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/usememos/memos/core/access"
	"github.com/usememos/memos/core/memopayload"
	"github.com/usememos/memos/internal/ratelimit"
	apipb "github.com/usememos/memos/proto/gen/api"
	storepb "github.com/usememos/memos/proto/gen/store"
	"github.com/usememos/memos/store"
)

const maxBatchGetLinkMetadata = 10

func (s *APIService) CreateMemo(ctx context.Context, request *apipb.CreateMemoRequest) (*apipb.Memo, error) {
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user")
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if err := s.throttleAndCharge(ratelimit.ScopeWriteUser, userKey(user.ID), 1); err != nil {
		return nil, err
	}
	if request.Memo == nil {
		return nil, status.Errorf(codes.InvalidArgument, "memo is required")
	}

	digest, err := memoCreationDigest(request.MemoId, request.Idempotent, request)
	if err != nil {
		return nil, err
	}
	memoUID, err := ValidateAndGenerateUID(request.MemoId)
	if err != nil {
		return nil, err
	}

	memo, replay, err := s.saveNewMemo(ctx, user, request.Memo, memoUID, digest, nil)
	if replay != nil || err != nil {
		return replay, err
	}

	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{
		MemoID: &memo.ID,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get memo attachments")
	}

	relations, err := s.loadMemoRelations(ctx, memo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load memo relations")
	}
	memoMessage, err := s.convertMemoFromStore(ctx, memo, nil, attachments, relations)
	if err != nil {
		return nil, errors.Wrap(err, "failed to convert memo")
	}
	return memoMessage, nil
}

func (s *APIService) ListMemos(ctx context.Context, request *apipb.ListMemosRequest) (*apipb.ListMemosResponse, error) {
	return s.listMemoPage(ctx, request, 0)
}

// ListMCPMemos applies a Markdown byte budget to each MCP response page.
func (s *APIService) ListMCPMemos(ctx context.Context, request *apipb.ListMemosRequest) (*apipb.ListMemosResponse, error) {
	return s.listMemoPage(ctx, request, 4<<20)
}

func (s *APIService) listMemoPage(ctx context.Context, request *apipb.ListMemosRequest, contentBudget int) (*apipb.ListMemosResponse, error) {
	memoFind := &store.FindMemo{
		// Exclude comments by default.
		ExcludeComments: true,
	}
	accessScope, currentUser, err := s.resolveMemoAccessScope(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	if currentUser == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	memoFind.Access = accessScope

	if request.State == apipb.State_ARCHIVED {
		state := store.Archived
		memoFind.RowStatus = &state
	} else {
		state := store.Normal
		memoFind.RowStatus = &state
	}

	// Parse order_by field (replaces the old sort and direction fields)
	if request.OrderBy != "" {
		if err := s.parseMemoOrderBy(request.OrderBy, memoFind); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid order_by: %v", err)
		}
	} else {
		// Default ordering by create_time desc.
		memoFind.OrderByTimeAsc = false
	}

	if request.Filter != "" {
		prepared, err := prepareMemoFilter(ctx, request.Filter)
		if err != nil {
			return nil, err
		}
		memoFind.PreparedFilter = prepared
	}

	var limit, offset int
	if request.PageToken != "" {
		var pageToken apipb.PageToken
		if err := unmarshalPageToken(request.PageToken, &pageToken); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid page token: %v", err)
		}
		limit = normalizePageSize(pageToken.Limit)
		offset = max(int(pageToken.Offset), 0)
	} else {
		limit = normalizePageSize(request.PageSize)
	}
	limit = min(limit, MaxPageSize)
	limitPlusOne := limit + 1
	memoFind.Limit = &limitPlusOne
	memoFind.Offset = &offset
	page, err := s.listMemoContentPage(ctx, memoFind, limit, contentBudget)
	if err != nil {
		if errors.Is(err, store.ErrMemoContentBudgetExceeded) || errors.Is(err, store.ErrMemoPageChanged) {
			return nil, status.Error(codes.Aborted, "memo content changed while reading the page; retry the request")
		}
		return nil, status.Errorf(codes.Internal, "failed to list memos: %v", err)
	}

	memos := page.Memos
	memoMessages := []*apipb.Memo{}
	nextPageToken := ""
	if page.HasMore {
		nextPageToken, err = getPageToken(limit, offset+page.Consumed)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get next page token, error: %v", err)
		}
	}
	if page.OversizedUID != "" {
		message := "memo " + MemoNamePrefix + page.OversizedUID + " exceeds 4 MiB of Markdown; read it with memo_get_memo"
		if nextPageToken != "" {
			message += ". Resume this list with pageToken " + nextPageToken
		}
		return nil, status.Error(codes.ResourceExhausted, message)
	}

	if len(memos) == 0 {
		response := &apipb.ListMemosResponse{
			Memos:         memoMessages,
			NextPageToken: nextPageToken,
		}
		return response, nil
	}

	reactionMap := make(map[int32][]*store.Reaction)

	attachmentMap := make(map[int32][]*store.Attachment)
	memoIDs := make([]int32, 0, len(memos))

	for _, m := range memos {
		memoIDs = append(memoIDs, m.ID)
	}

	// REACTIONS
	reactions, err := s.Store.ListReactions(ctx, &store.FindReaction{MemoIDList: memoIDs})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list reactions")
	}
	for _, reaction := range reactions {
		reactionMap[reaction.MemoID] = append(reactionMap[reaction.MemoID], reaction)
	}

	// ATTACHMENTS
	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{MemoIDList: memoIDs})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list attachments")
	}
	for _, attachment := range attachments {
		attachmentMap[*attachment.MemoID] = append(attachmentMap[*attachment.MemoID], attachment)
	}

	// RELATIONS (batch load to avoid N+1)
	relationMap, err := s.batchConvertMemoRelations(ctx, memos, false)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to batch load memo relations")
	}
	creatorIDs := make([]int32, 0, len(memos)+len(reactions))
	for _, memo := range memos {
		creatorIDs = append(creatorIDs, memo.CreatorID)
	}
	for _, reaction := range reactions {
		creatorIDs = append(creatorIDs, reaction.CreatorID)
	}
	creatorMap, err := s.listUsersByID(ctx, creatorIDs)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list memo creators: %v", err)
	}
	for _, memo := range memos {
		reactions := reactionMap[memo.ID]
		attachments := attachmentMap[memo.ID]
		relations := relationMap[memo.ID]

		memoMessage, err := s.convertMemoFromStoreWithCreators(ctx, memo, reactions, attachments, relations, creatorMap)
		if err != nil {
			if stderrors.Is(err, errMemoCreatorNotFound) {
				slog.Warn("Skipping memo with missing creator",
					slog.Int64("memo_id", int64(memo.ID)),
					slog.String("memo_uid", memo.UID),
					slog.Int64("creator_id", int64(memo.CreatorID)),
				)
				continue
			}
			return nil, errors.Wrap(err, "failed to convert memo")
		}

		memoMessages = append(memoMessages, memoMessage)
	}

	response := &apipb.ListMemosResponse{
		Memos:         memoMessages,
		NextPageToken: nextPageToken,
	}
	return response, nil
}

func (s *APIService) listMemoContentPage(ctx context.Context, find *store.FindMemo, limit, budget int) (store.MemoContentPage, error) {
	if budget > 0 {
		return s.Store.ListMemoContentPage(ctx, find, limit, budget)
	}
	memos, err := s.Store.ListMemos(ctx, find)
	if err != nil {
		return store.MemoContentPage{}, err
	}
	count := min(limit, len(memos))
	return store.MemoContentPage{Memos: memos[:count], Consumed: count, HasMore: len(memos) > limit}, nil
}

func (s *APIService) GetMemo(ctx context.Context, request *apipb.GetMemoRequest) (*apipb.Memo, error) {
	memoUID, err := ExtractMemoUIDFromName(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid memo name: %v", err)
	}
	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{
		UID: &memoUID,
	})
	if err != nil {
		return nil, err
	}
	if memo == nil {
		return nil, status.Errorf(codes.NotFound, "memo not found")
	}
	return s.readMemoResponse(ctx, memo)
}

func (s *APIService) readMemoResponse(ctx context.Context, memo *store.Memo) (*apipb.Memo, error) {
	if err := s.checkMemoReadAccess(ctx, memo); err != nil {
		return nil, err
	}

	reactions, err := s.Store.ListReactions(ctx, &store.FindReaction{
		MemoID: &memo.ID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list reactions")
	}

	attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{
		MemoID: &memo.ID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list attachments")
	}

	relations, err := s.loadMemoRelations(ctx, memo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to load memo relations")
	}
	memoMessage, err := s.convertMemoFromStore(ctx, memo, reactions, attachments, relations)
	if err != nil {
		if stderrors.Is(err, errMemoCreatorNotFound) {
			return nil, status.Errorf(codes.NotFound, "memo creator not found")
		}
		return nil, errors.Wrap(err, "failed to convert memo")
	}
	return memoMessage, nil
}

// UpdateMemo updates an existing memo.
func (s *APIService) UpdateMemo(ctx context.Context, request *apipb.UpdateMemoRequest) (*apipb.Memo, error) {
	if request.Memo == nil {
		return nil, status.Errorf(codes.InvalidArgument, "memo is required")
	}
	memoUID, err := ExtractMemoUIDFromName(request.Memo.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid memo name: %v", err)
	}
	if request.UpdateMask == nil || len(request.UpdateMask.Paths) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "update mask is required")
	}

	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{UID: &memoUID})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get memo: %v", err)
	}
	if memo == nil {
		return nil, status.Errorf(codes.NotFound, "memo not found")
	}

	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if !access.CanManageMemo(user, memo) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	if request.Memo.GetSpace() != "" {
		return nil, retiredModuleError()
	}

	update := &store.UpdateMemo{
		ID: memo.ID,
	}
	if expected := request.GetExpectedRevision(); expected != "" {
		if expected != MemoRevision(memo) {
			return nil, mapMemoWriteError(store.ErrMemoRevisionConflict, "failed to update memo")
		}
		update.ExpectedRevision = &memo.Revision
	}
	updatePaths := make(map[string]bool, len(request.UpdateMask.Paths))
	for _, path := range request.UpdateMask.Paths {
		// Older clients can include the retired Space field in the mask. Locations are read-only.
		if path == "space" || path == "location" {
			continue
		}
		updatePaths[path] = true
	}
	if len(updatePaths) == 0 {
		return s.GetMemo(ctx, &apipb.GetMemoRequest{Name: request.Memo.Name})
	}
	if updatePaths["visibility"] {
		nextVisibility, err := validateUpdateMemoVisibility(request.Memo.Visibility)
		if err != nil {
			return nil, err
		}
		update.Visibility = &nextVisibility
	}
	update.Policy = &store.MemoWritePolicy{ActorUserID: user.ID}
	contentUpdated := false
	attachmentsUpdated := false
	relationsUpdated := false
	nextMemo := *memo
	if memo.Payload != nil {
		nextMemo.Payload = &storepb.MemoPayload{}
		proto.Merge(nextMemo.Payload, memo.Payload)
	}

	for _, path := range request.UpdateMask.Paths {
		// Placement and legacy visibility are handled above. Location is read-only.
		if path == "visibility" || path == "space" || path == "location" {
			continue
		}
		if path == "content" {
			contentUpdated = true
			nextMemo.Content = request.Memo.Content
			if err := memopayload.RebuildMemoPayload(ctx, &nextMemo, s.MarkdownService); err != nil {
				return nil, status.Errorf(codes.Internal, "failed to rebuild memo payload: %v", err)
			}
			update.Content = &nextMemo.Content
			update.Payload = nextMemo.Payload
		} else if path == "pinned" {
			update.Pinned = &request.Memo.Pinned
		} else if path == "state" {
			rowStatus := convertStateToStore(request.Memo.State)
			update.RowStatus = &rowStatus
		} else if path == "create_time" {
			if request.Memo.CreateTime == nil || !request.Memo.CreateTime.IsValid() {
				return nil, status.Errorf(codes.InvalidArgument, "create_time is invalid")
			}
			createdTs := request.Memo.CreateTime.AsTime().Unix()
			update.CreatedTs = &createdTs
		} else if path == "update_time" {
			updatedTsSec := time.Now().Unix()
			if request.Memo.UpdateTime != nil {
				if !request.Memo.UpdateTime.IsValid() {
					return nil, status.Errorf(codes.InvalidArgument, "update_time is invalid")
				}
				updatedTsSec = request.Memo.UpdateTime.AsTime().Unix()
			}
			update.UpdatedTs = &updatedTsSec
		} else if path == "display_time" {
			return nil, status.Errorf(codes.InvalidArgument, "display_time is not supported")
		} else if path == "attachments" {
			attachmentsUpdated = true
		} else if path == "relations" {
			relationsUpdated = true
		} else {
			return nil, status.Errorf(codes.InvalidArgument, "invalid update path: %s", path)
		}
	}

	var preparedAttachments *preparedMemoAttachments
	if attachmentsUpdated {
		preparedAttachments, err = s.prepareMemoAttachments(ctx, user, memo, request.Memo.Attachments)
		if err != nil {
			return nil, err
		}
	}
	var preparedRelations []*store.MemoRelation
	if relationsUpdated {
		preparedRelations, err = s.prepareMemoRelations(ctx, memo, request.Memo.Relations)
		if err != nil {
			return nil, err
		}
	}
	var requiredAttachmentIDs []int32
	if contentUpdated || attachmentsUpdated {
		var finalAttachments []*store.Attachment
		if preparedAttachments != nil {
			finalAttachments = preparedAttachments.normalized
		} else {
			finalAttachments, err = s.Store.ListAttachments(ctx, &store.FindAttachment{MemoID: &memo.ID})
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to list attachments")
			}
		}
		requiredAttachmentIDs, err = s.resolveMemoAttachmentReferences(nextMemo.Content, finalAttachments)
		if err != nil {
			return nil, err
		}
	}

	if contentUpdated || attachmentsUpdated || relationsUpdated {
		var relations *[]*store.MemoRelation
		if relationsUpdated {
			relations = &preparedRelations
		}
		if err := s.applyMemoMutation(ctx, memo, preparedAttachments, update, requiredAttachmentIDs, relations); err != nil {
			return nil, err
		}
	} else if err = s.Store.UpdateMemo(ctx, update); err != nil {
		return nil, mapMemoWriteError(err, "failed to update memo")
	}

	memoMessage, err := s.loadUpdatedMemo(ctx, memo.ID)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build updated memo state")
	}
	s.SSEHub.publishMemoChanged()
	return memoMessage, nil
}

func (s *APIService) DeleteMemo(ctx context.Context, request *apipb.DeleteMemoRequest) (*emptypb.Empty, error) {
	memoUID, err := ExtractMemoUIDFromName(request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid memo name: %v", err)
	}
	memo, err := s.Store.GetMemo(ctx, &store.FindMemo{
		UID: &memoUID,
	})
	if err != nil {
		return nil, err
	}
	if memo == nil {
		return nil, status.Errorf(codes.NotFound, "memo not found")
	}

	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get current user")
	}
	if user == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if !access.CanManageMemo(user, memo) {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	_, err = s.Store.ApplyMemoTrash(ctx, &store.MemoTrashMutation{
		MemoID: memo.ID, ActorUserID: user.ID, Action: store.MemoTrashMove, NowTs: time.Now().Unix(),
	})
	if err != nil {
		switch {
		case stderrors.Is(err, store.ErrMemoPermissionDenied):
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		case stderrors.Is(err, store.ErrMemoMutationConflict):
			return nil, status.Error(codes.FailedPrecondition, "memo changed or no longer exists")
		default:
			return nil, status.Errorf(codes.Internal, "failed to delete memo: %v", err)
		}
	}
	s.SSEHub.publishMemoChanged()
	return &emptypb.Empty{}, nil
}
