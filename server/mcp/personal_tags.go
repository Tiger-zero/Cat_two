package mcp

import (
	"context"
	"net/http"
	"slices"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pkg/errors"
	"golang.org/x/text/cases"

	"github.com/usememos/memos/store"
)

const (
	defaultTagPageSize = 50
	maxTagPageSize     = 200
	tagMemoBatchSize   = 500
)

type personalTagPagination struct {
	Offset   int `json:"offset,omitempty" jsonschema:"Zero-based offset into the sorted tag results."`
	PageSize int `json:"pageSize,omitempty" jsonschema:"Maximum number of tags to return. Defaults to 50 and is capped at 200."`
}

type personalTagSearchInput struct {
	personalTagPagination
	Query string `json:"query" jsonschema:"Nonempty text to match case-insensitively anywhere in the tag path. An initial # is optional."`
}

type personalTag struct {
	Path      string `json:"path"`
	Parent    string `json:"parent,omitempty"`
	MemoCount int    `json:"memoCount"`
}

type personalTagResults struct {
	Tags       []personalTag `json:"tags"`
	Total      int           `json:"total"`
	NextOffset int           `json:"nextOffset,omitempty"`
}

func registerPersonalTagTools(server *sdkmcp.Server, db *store.Store) {
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "tag_tree",
		Title:       "List Tag Paths",
		Description: "List your active notes' tag paths and their parents, in path order. Count each note once per tag, including notes tagged with any descendant.",
		Annotations: annotationsForMethod(http.MethodGet, "List Tag Paths"),
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input personalTagPagination) (*sdkmcp.CallToolResult, personalTagResults, error) {
		result, err := listPersonalTags(ctx, db, "", input)
		return nil, result, err
	})
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "tag_search",
		Title:       "Search Tag Paths",
		Description: "Search your active notes' tag paths by case-insensitive substring. Counts include notes tagged with descendants.",
		Annotations: annotationsForMethod(http.MethodGet, "Search Tag Paths"),
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input personalTagSearchInput) (*sdkmcp.CallToolResult, personalTagResults, error) {
		query := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input.Query), "#"))
		if query == "" {
			return nil, personalTagResults{}, errors.New("query must not be empty")
		}
		result, err := listPersonalTags(ctx, db, query, input.personalTagPagination)
		return nil, result, err
	})
}

func listPersonalTags(ctx context.Context, db *store.Store, query string, page personalTagPagination) (personalTagResults, error) {
	userID, ok := ctx.Value(connectionUserIDContextKey{}).(int32)
	if !ok || userID <= 0 {
		return personalTagResults{}, errors.New("an authenticated MCP connection is required")
	}
	if page.Offset < 0 || page.PageSize < 0 {
		return personalTagResults{}, errors.New("offset and pageSize must be nonnegative")
	}
	pageSize := min(page.PageSize, maxTagPageSize)
	if pageSize == 0 {
		pageSize = defaultTagPageSize
	}

	counts, err := personalTagMemoCounts(ctx, db, userID)
	if err != nil {
		return personalTagResults{}, errors.Wrap(err, "failed to list tags")
	}
	fold := cases.Fold()
	query = fold.String(query)
	var paths []string
	for path := range counts {
		if query == "" || strings.Contains(fold.String(path), query) {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)

	start := min(page.Offset, len(paths))
	end := min(start+pageSize, len(paths))
	result := personalTagResults{Tags: make([]personalTag, 0, end-start), Total: len(paths)}
	for _, path := range paths[start:end] {
		tag := personalTag{Path: path, MemoCount: counts[path]}
		if index := strings.LastIndexByte(path, '/'); index >= 0 {
			tag.Parent = path[:index]
		}
		result.Tags = append(result.Tags, tag)
	}
	if end < len(paths) {
		result.NextOffset = end
	}
	return result, nil
}

func personalTagMemoCounts(ctx context.Context, db *store.Store, userID int32) (map[string]int, error) {
	normal, limit := store.Normal, tagMemoBatchSize
	find := store.FindMemo{
		Access:          &store.MemoAccessScope{UserID: &userID, OwnerOnly: true},
		RowStatus:       &normal,
		Trash:           store.MemoTrashExclude,
		ExcludeComments: true,
		ExcludeContent:  true,
		Limit:           &limit,
	}

	counts := make(map[string]int)
	for offset := 0; ; offset += limit {
		find.Offset = &offset
		memos, err := db.ListMemos(ctx, &find)
		if err != nil {
			return nil, err
		}
		for _, memo := range memos {
			countMemoTagPaths(counts, memo.Payload.GetTags())
		}
		if len(memos) < limit {
			return counts, nil
		}
	}
}

func countMemoTagPaths(counts map[string]int, tags []string) {
	seen := make(map[string]struct{}, len(tags))
	for _, path := range tags {
		// New payloads already record ancestor paths. Expanding them also
		// handles older payloads, while seen keeps branches from double-counting.
		for path != "" {
			if _, exists := seen[path]; exists {
				break
			}
			seen[path] = struct{}{}
			counts[path]++
			index := strings.LastIndexByte(path, '/')
			if index < 0 {
				break
			}
			path = path[:index]
		}
	}
}
