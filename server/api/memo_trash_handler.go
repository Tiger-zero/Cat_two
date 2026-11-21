package api

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/usememos/memos/core/access"
	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/internal/ratelimit"
	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

type trashMemoJSON struct {
	Memo        json.RawMessage `json:"memo"`
	DeletedTime time.Time       `json:"deletedTime"`
}

type memoTrashPage struct {
	Memos         []trashMemoJSON `json:"memos"`
	NextPageToken string          `json:"nextPageToken"`
}

func (s *APIService) newMemoTrashHandler(authorizer *Authorizer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/trash", s.authorizeMemoTrash(authorizer, func(w http.ResponseWriter, r *http.Request, user *store.User) error {
		page, err := s.listMemoTrash(r.Context(), user, r.URL.Query().Get("pageSize"), r.URL.Query().Get("pageToken"), time.Now())
		if err != nil {
			return err
		}
		return httphandler.JSON(w, http.StatusOK, page)
	}))
	mux.HandleFunc("POST /api/trash/{uid}/restore", s.authorizeMemoTrash(authorizer, s.handleMemoTrashMutation(store.MemoTrashRestore)))
	mux.HandleFunc("DELETE /api/trash/{uid}", s.authorizeMemoTrash(authorizer, s.handleMemoTrashMutation(store.MemoTrashPurge)))
	return mux
}

func (s *APIService) authorizeMemoTrash(authorizer *Authorizer, handler func(http.ResponseWriter, *http.Request, *store.User) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setAPIResponseNoStoreHeaders(w.Header())
		ctx := r.Context()
		result := authorizer.Authenticate(ctx, r.Header.Get("Authorization"))
		if err := authorizer.CheckAccess(ctx, "", result); err != nil {
			writeGatewayAuthorizationError(w, err)
			return
		}
		if err := authorizer.Throttle(ctx, "", result); err != nil {
			writeGatewayStatusError(w, err)
			return
		}
		r = r.WithContext(auth.ApplyToContext(ctx, result))
		user, err := s.fetchCurrentUser(r.Context())
		if err != nil {
			writeGatewayStatusError(w, status.Error(codes.Internal, "failed to get current user"))
			return
		}
		if !access.IsActiveUser(user) {
			writeGatewayStatusError(w, status.Error(codes.Unauthenticated, "authentication required"))
			return
		}
		if err := handler(w, r, user); err != nil {
			writeGatewayStatusError(w, err)
		}
	}
}

func (s *APIService) listMemoTrash(ctx context.Context, user *store.User, pageSize, pageToken string, now time.Time) (*memoTrashPage, error) {
	var size int64
	var offset int
	if pageSize != "" {
		parsed, err := strconv.ParseInt(pageSize, 10, 32)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid page size")
		}
		size = parsed
	}
	if pageToken != "" {
		var page apipb.PageToken
		if err := unmarshalPageToken(pageToken, &page); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid page token")
		}
		size = int64(page.Limit)
		offset = max(int(page.Offset), 0)
	}
	limit := normalizePageSize(int32(size))
	itemsToRead := limit + 1
	deletedAfter := now.Add(-store.MemoTrashRetention).Unix()
	memos, err := s.Store.ListMemos(ctx, &store.FindMemo{
		Trash: store.MemoTrashOnly, CreatorID: &user.ID, DeletedAfterTs: &deletedAfter, Limit: &itemsToRead, Offset: &offset,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to list deleted memos")
	}
	page := &memoTrashPage{Memos: make([]trashMemoJSON, 0, min(limit, len(memos)))}
	if len(memos) > limit {
		memos = memos[:limit]
		page.NextPageToken, err = getPageToken(limit, offset+limit)
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to encode next page token")
		}
	}
	creators := map[int32]*store.User{user.ID: user}
	attachmentsByMemo := make(map[int32][]*store.Attachment, len(memos))
	if len(memos) > 0 {
		memoIDs := make([]int32, 0, len(memos))
		for _, memo := range memos {
			memoIDs = append(memoIDs, memo.ID)
		}
		attachments, err := s.Store.ListAttachments(ctx, &store.FindAttachment{MemoIDList: memoIDs})
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to read deleted memo attachments")
		}
		for _, attachment := range attachments {
			attachmentsByMemo[*attachment.MemoID] = append(attachmentsByMemo[*attachment.MemoID], attachment)
		}
	}
	for _, memo := range memos {
		message, err := s.convertMemoFromStoreWithCreators(ctx, memo, nil, attachmentsByMemo[memo.ID], nil, creators)
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to read deleted memo")
		}
		encoded, err := (protojson.MarshalOptions{EmitDefaultValues: true}).Marshal(message)
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to encode deleted memo")
		}
		page.Memos = append(page.Memos, trashMemoJSON{Memo: encoded, DeletedTime: time.Unix(*memo.DeletedTs, 0).UTC()})
	}
	return page, nil
}

func (s *APIService) handleMemoTrashMutation(action store.MemoTrashAction) func(http.ResponseWriter, *http.Request, *store.User) error {
	return func(w http.ResponseWriter, r *http.Request, user *store.User) error {
		uid, err := ExtractMemoUIDFromName(MemoNamePrefix + r.PathValue("uid"))
		if err != nil {
			return status.Error(codes.InvalidArgument, "invalid memo name")
		}
		if err := s.throttleAndCharge(ratelimit.ScopeWriteUser, userKey(user.ID), 1); err != nil {
			return err
		}
		ctx := r.Context()
		memo, err := s.Store.GetMemo(ctx, &store.FindMemo{UID: &uid, CreatorID: &user.ID, Trash: store.MemoTrashOnly, ExcludeContent: true})
		if err != nil {
			return status.Error(codes.Internal, "failed to read deleted memo")
		}
		if memo == nil {
			return status.Error(codes.NotFound, "deleted memo not found")
		}
		now := time.Now()
		attachments, err := s.Store.ApplyMemoTrash(ctx, &store.MemoTrashMutation{
			MemoID: memo.ID, ActorUserID: user.ID, Action: action, NowTs: now.Unix(),
		})
		if err != nil {
			return mapMemoWriteError(err, "failed to change deleted memo")
		}
		s.SSEHub.publishMemoChanged()
		if len(attachments) != 0 {
			// Storage cleanup can fail after the memo is gone. The jobs are
			// already durable, so the response reports the committed purge.
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			s.cleanupPurgedMemoAttachments(cleanupCtx, attachments, now)
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
}

func (s *APIService) cleanupPurgedMemoAttachments(ctx context.Context, attachments []*store.Attachment, now time.Time) {
	for _, attachment := range attachments {
		if ctx.Err() != nil {
			return
		}
		if err := s.Store.RunMemoTrashAttachmentCleanup(ctx, attachment, now); err != nil && ctx.Err() == nil {
			slog.Error("failed to clean purged memo attachment; retrying", "attachment", attachment.ID, "error", err)
		}
	}
}

// PurgeExpiredMemoTrash removes expired entries and retries attachment cleanup.
func (s *APIService) PurgeExpiredMemoTrash(ctx context.Context, now time.Time) error {
	purgeErr := s.purgeExpiredTrashMemos(ctx, now)
	return stderrors.Join(purgeErr, s.retryPurgedMemoAttachments(ctx, now))
}

func (s *APIService) retryPurgedMemoAttachments(ctx context.Context, now time.Time) error {
	const batchSize = 100
	var afterID int32
	for {
		attachments, err := s.Store.ListMemoTrashAttachmentCleanup(ctx, afterID, now, batchSize)
		if err != nil {
			return err
		}
		s.cleanupPurgedMemoAttachments(ctx, attachments, now)
		if len(attachments) < batchSize {
			return ctx.Err()
		}
		afterID = attachments[len(attachments)-1].ID
	}
}

func (s *APIService) purgeExpiredTrashMemos(ctx context.Context, now time.Time) error {
	const batchSize = 100
	cutoff := now.Add(-store.MemoTrashRetention).Unix()
	limit := batchSize
	changed := false
	defer func() {
		if changed {
			s.SSEHub.publishMemoChanged()
		}
	}()
	for {
		memos, err := s.Store.ListMemos(ctx, &store.FindMemo{
			Trash: store.MemoTrashOnly, DeletedThroughTs: &cutoff, ExcludeContent: true, Limit: &limit, OrderByTimeAsc: true,
		})
		if err != nil {
			return err
		}
		for _, memo := range memos {
			_, err := s.Store.ApplyMemoTrash(ctx, &store.MemoTrashMutation{
				MemoID: memo.ID, Action: store.MemoTrashPurge, NowTs: now.Unix(),
			})
			if stderrors.Is(err, store.ErrMemoMutationConflict) {
				continue
			}
			if err != nil {
				return err
			}
			changed = true
		}
		if len(memos) < batchSize {
			return nil
		}
	}
}
