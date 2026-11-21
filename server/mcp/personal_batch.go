package mcp

import (
	"context"
	"errors"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usememos/memos/internal/identifier"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/store"
)

const maxMemoBatchNames = 50

type personalMemoBatchInput struct {
	Names []string `json:"names"`
}

type personalMemoBatchEntry struct {
	Name            string          `json:"name"`
	Content         string          `json:"content"`
	Tags            []string        `json:"tags"`
	CreateTime      time.Time       `json:"createTime"`
	UpdateTime      time.Time       `json:"updateTime"`
	Pinned          bool            `json:"pinned"`
	State           store.RowStatus `json:"state"`
	CreationChannel string          `json:"creationChannel"`
}

type personalMemoBatchResult struct {
	Memos       []personalMemoBatchEntry `json:"memos"`
	Unavailable []string                 `json:"unavailable"`
}

func registerPersonalMemoBatchGet(server *sdkmcp.Server, db *store.Store) {
	const title = "Get Memos in Batch"
	namePattern := "^" + api.MemoNamePrefix + strings.TrimPrefix(identifier.UIDMatcher.String(), "^")
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:        "memo_batch_get",
		Title:       title,
		Description: "Read full Markdown and basic metadata for up to 50 of your memos, including archived memos. Unreadable names are listed in unavailable. Use memo_get_memo for attachment details.",
		Annotations: annotationsForMethod("GET", title),
		InputSchema: jsonSchema{
			"type": "object",
			"properties": jsonSchema{
				"names": jsonSchema{
					"type":     "array",
					"minItems": 1,
					"maxItems": maxMemoBatchNames,
					"items":    jsonSchema{"type": "string", "pattern": namePattern},
				},
			},
			"required":             []string{"names"},
			"additionalProperties": false,
		},
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input personalMemoBatchInput) (*sdkmcp.CallToolResult, personalMemoBatchResult, error) {
		userID, ok := ctx.Value(connectionUserIDContextKey{}).(int32)
		if !ok || userID <= 0 {
			return nil, personalMemoBatchResult{}, errors.New("MCP authentication required")
		}
		if len(input.Names) == 0 || len(input.Names) > maxMemoBatchNames {
			return nil, personalMemoBatchResult{}, errors.New("expected 1 to 50 memo resource names")
		}

		uids := make([]string, 0, len(input.Names))
		seen := make(map[string]struct{}, len(input.Names))
		for _, name := range input.Names {
			uid, ok := strings.CutPrefix(name, api.MemoNamePrefix)
			if !ok || !identifier.UIDMatcher.MatchString(uid) {
				return nil, personalMemoBatchResult{}, errors.New("invalid memo resource name")
			}
			if _, duplicate := seen[uid]; duplicate {
				continue
			}
			seen[uid] = struct{}{}
			uids = append(uids, uid)
		}
		memos, err := db.ListMemos(ctx, &store.FindMemo{
			UIDList:   uids,
			CreatorID: &userID,
			Access:    &store.MemoAccessScope{OwnerOnly: true, UserID: &userID},
			Trash:     store.MemoTrashExclude,
		})
		if err != nil {
			return nil, personalMemoBatchResult{}, errors.New("failed to read memo batch")
		}
		byUID := make(map[string]*store.Memo, len(memos))
		for _, memo := range memos {
			byUID[memo.UID] = memo
		}
		result := personalMemoBatchResult{
			Memos:       make([]personalMemoBatchEntry, 0, len(memos)),
			Unavailable: []string{},
		}
		for _, uid := range uids {
			name := api.MemoNamePrefix + uid
			memo := byUID[uid]
			if memo == nil {
				result.Unavailable = append(result.Unavailable, name)
				continue
			}
			tags := memo.Payload.GetTags()
			if tags == nil {
				tags = []string{}
			}
			channel := memo.Payload.GetCreationChannel()
			if channel == "" {
				channel = store.MemoCreationChannelUnknown
			}
			result.Memos = append(result.Memos, personalMemoBatchEntry{
				Name:            name,
				Content:         memo.Content,
				Tags:            tags,
				CreateTime:      time.Unix(memo.CreatedTs, 0).UTC(),
				UpdateTime:      time.Unix(memo.UpdatedTs, 0).UTC(),
				Pinned:          memo.Pinned,
				State:           memo.RowStatus,
				CreationChannel: channel,
			})
		}
		return nil, result, nil
	})
}
