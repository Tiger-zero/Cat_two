package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/usememos/memos/core/memopayload"
	markdownparser "github.com/usememos/memos/markdown/parser"
	"github.com/usememos/memos/store"
)

func (s *APIService) registerMemoTagRoutes(mux *runtime.ServeMux) error {
	if err := mux.HandlePath(http.MethodPost, "/api/tags/rename", s.renameMemoTag); err != nil {
		return err
	}
	return mux.HandlePath(http.MethodPost, "/api/tags/normalize", s.normalizeMemoTags)
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
	s.applyTagEdit(w, r, "tag in ["+strconv.Quote(oldTag)+"]", func(content []byte) (string, error) {
		return s.MarkdownService.RenameTagBranch(content, oldTag, newTag)
	})
}

func (s *APIService) normalizeMemoTags(w http.ResponseWriter, r *http.Request, _ map[string]string) {
	s.applyTagEdit(w, r, "", s.MarkdownService.NormalizeTagWhitespace)
}

func (s *APIService) applyTagEdit(w http.ResponseWriter, r *http.Request, filter string, edit func([]byte) (string, error)) {
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
	count, err := s.editOwnMemoTags(ctx, user.ID, filter, edit)
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
		MemoUpdate:          &store.UpdateMemo{ID: memo.ID, Content: &next.Content, Payload: next.Payload, UpdatedTs: &memo.UpdatedTs},
		Policy:              memoWritePolicy(memo.CreatorID, false),
	})
	return err == nil, err
}
