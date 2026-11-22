package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/usememos/memos/core/memopayload"
	"github.com/usememos/memos/filter"
	markdownparser "github.com/usememos/memos/markdown/parser"
	"github.com/usememos/memos/store"
)

func (s *APIService) registerMemoTagRoutes(mux *runtime.ServeMux) error {
	if err := mux.HandlePath(http.MethodPost, "/api/tags/rename", s.renameMemoTag); err != nil {
		return err
	}
	if err := mux.HandlePath(http.MethodPost, "/api/tags/normalize", s.normalizeMemoTags); err != nil {
		return err
	}
	return mux.HandlePath(http.MethodPost, "/api/tags/delete", s.deleteMemoTag)
}

func validTagPath(tag string) bool {
	source := []byte("#" + tag)
	matches := markdownparser.FindTagMatches(source)
	return len(matches) == 1 && matches[0].Start == 0 && matches[0].End == len(source) && string(matches[0].Value) == tag
}

func tagPathInput(input string) string {
	tag := strings.TrimSpace(input)
	if validTagPath(tag) {
		return tag
	}
	return strings.TrimPrefix(tag, "#")
}

func (s *APIService) renameMemoTag(w http.ResponseWriter, r *http.Request, _ map[string]string) {
	var request struct {
		OldName string `json:"oldName"`
		NewName string `json:"newName"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil || decoder.Decode(new(any)) != io.EOF {
		writeGatewayStatusError(w, status.Error(codes.InvalidArgument, "expected tag rename request"))
		return
	}
	oldTag := tagPathInput(request.OldName)
	newTag := tagPathInput(request.NewName)
	if !validTagPath(oldTag) || !validTagPath(newTag) || strings.HasPrefix(newTag, oldTag+"/") {
		writeGatewayStatusError(w, status.Error(codes.InvalidArgument, "enter a valid tag path outside the original branch"))
		return
	}
	s.applyTagEdit(w, r, filter.TagBranchExpression(oldTag), func(content []byte) (string, error) {
		return s.MarkdownService.RenameTagBranch(content, oldTag, newTag)
	})
}

func (s *APIService) normalizeMemoTags(w http.ResponseWriter, r *http.Request, _ map[string]string) {
	s.applyTagEdit(w, r, "", s.MarkdownService.NormalizeTagWhitespace)
}

func (s *APIService) deleteMemoTag(w http.ResponseWriter, r *http.Request, _ map[string]string) {
	var request struct {
		TagName string `json:"tagName"`
		Mode    string `json:"mode"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil || decoder.Decode(new(any)) != io.EOF {
		writeGatewayStatusError(w, status.Error(codes.InvalidArgument, "expected tag deletion request"))
		return
	}
	tag := tagPathInput(request.TagName)
	if !validTagPath(tag) {
		writeGatewayStatusError(w, status.Error(codes.InvalidArgument, "enter a valid tag path"))
		return
	}
	branchFilter := filter.TagBranchExpression(tag)
	switch request.Mode {
	case "remove":
		s.applyTagEdit(w, r, branchFilter, func(content []byte) (string, error) {
			return s.MarkdownService.RemoveTagBranch(content, tag)
		})
	case "trash":
		s.applyTagMutation(w, r, func(ctx context.Context, userID int32) (int, error) {
			return s.trashOwnTaggedMemos(ctx, userID, branchFilter)
		})
	default:
		writeGatewayStatusError(w, status.Error(codes.InvalidArgument, "choose remove or trash for tag deletion"))
	}
}

func (s *APIService) applyTagEdit(w http.ResponseWriter, r *http.Request, filter string, edit func([]byte) (string, error)) {
	s.applyTagMutation(w, r, func(ctx context.Context, userID int32) (int, error) {
		return s.editOwnMemoTags(ctx, userID, filter, edit)
	})
}

func (s *APIService) applyTagMutation(w http.ResponseWriter, r *http.Request, mutate func(context.Context, int32) (int, error)) {
	ctx := r.Context()
	user, err := s.fetchCurrentUser(ctx)
	if err != nil {
		writeGatewayStatusError(w, status.Error(codes.Internal, "failed to get user"))
		return
	}
	if user == nil {
		writeGatewayStatusError(w, status.Error(codes.Unauthenticated, "authentication required"))
		return
	}
	count, err := mutate(ctx, user.ID)
	if count > 0 {
		s.SSEHub.publishMemoChanged()
	}
	if err != nil {
		writeGatewayStatusError(w, mapMemoWriteError(err, "failed to edit tags"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]int{"updatedCount": count})
}

func (s *APIService) trashOwnTaggedMemos(ctx context.Context, userID int32, filter string) (int, error) {
	memos, err := s.Store.ListMemos(ctx, &store.FindMemo{
		CreatorID: &userID, Filters: []string{filter}, ExcludeContent: true, ExcludeComments: true,
	})
	if err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	for index, memo := range memos {
		_, err := s.Store.ApplyMemoTrash(ctx, &store.MemoTrashMutation{
			MemoID: memo.ID, ActorUserID: userID, Action: store.MemoTrashMove, NowTs: now, ExpectedRevision: &memo.Revision,
		})
		if err != nil {
			return index, err
		}
	}
	return len(memos), nil
}

func (s *APIService) editOwnMemoTags(ctx context.Context, userID int32, filter string, edit func([]byte) (string, error)) (int, error) {
	find := &store.FindMemo{CreatorID: &userID, ExcludeContent: true}
	if filter != "" {
		find.Filters = []string{filter}
	}
	// Snapshot identifiers before any writes change the matching tags.
	matches, err := s.Store.ListMemos(ctx, find)
	if err != nil {
		return 0, err
	}
	ids := make([]int32, 0, len(matches))
	for _, memo := range matches {
		ids = append(ids, memo.ID)
	}
	count := 0
	for chunk := range slices.Chunk(ids, 64) {
		memos, err := s.Store.ListMemos(ctx, &store.FindMemo{CreatorID: &userID, IDList: chunk})
		if err != nil {
			return count, err
		}
		for _, memo := range memos {
			updated, err := s.editMemoTags(ctx, memo, edit)
			if err != nil {
				return count, err
			}
			if updated {
				count++
			}
		}
	}
	return count, nil
}

func (s *APIService) editMemoTags(ctx context.Context, memo *store.Memo, edit func([]byte) (string, error)) (bool, error) {
	updatedContent, err := edit([]byte(memo.Content))
	if err != nil {
		return false, err
	}
	if updatedContent == memo.Content {
		return false, nil
	}
	next := *memo
	next.Content = updatedContent
	next.Payload = proto.CloneOf(memo.Payload)
	if err := memopayload.RebuildMemoPayload(ctx, &next, s.MarkdownService); err != nil {
		return false, err
	}
	err = s.Store.ApplyMemoMutation(ctx, &store.MemoMutation{
		MemoID:              memo.ID,
		MemoCreatorID:       memo.CreatorID,
		ExpectedMemoContent: memo.Content,
		ExpectedRevision:    &memo.Revision,
		MemoUpdate:          &store.UpdateMemo{ID: memo.ID, Content: &next.Content, Payload: next.Payload, UpdatedTs: &memo.UpdatedTs},
		Policy:              s.memoWritePolicy(memo.CreatorID, false),
	})
	return err == nil, err
}
