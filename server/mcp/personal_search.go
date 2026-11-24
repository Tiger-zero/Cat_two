package mcp

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	"github.com/usememos/memos/filter"
	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/store"
)

const (
	defaultMemoSearchPageSize = 20
	maxMemoSearchPageSize     = 50
)

type memoSearchInput struct {
	Query              string `json:"query,omitempty" jsonschema:"Case-insensitive literal substring of the Markdown content. Omit to list without a text filter."`
	Tag                string `json:"tag,omitempty" jsonschema:"Tag path, with or without a leading #. Matches this tag and its descendants."`
	HasTags            *bool  `json:"hasTags,omitempty" jsonschema:"Whether the memo has any tags. Set false to find untagged notes."`
	StartDate          string `json:"startDate,omitempty" jsonschema:"Inclusive first creation date in Asia/Shanghai (YYYY-MM-DD)."`
	EndDate            string `json:"endDate,omitempty" jsonschema:"Inclusive last creation date in Asia/Shanghai (YYYY-MM-DD)."`
	HasIncompleteTasks *bool  `json:"hasIncompleteTasks,omitempty" jsonschema:"Whether the memo contains unchecked tasks."`
	HasAttachments     *bool  `json:"hasAttachments,omitempty" jsonschema:"Whether the memo has attached files."`
	Pinned             *bool  `json:"pinned,omitempty" jsonschema:"Whether the memo is pinned."`
	CreationChannel    string `json:"creationChannel,omitempty" jsonschema:"Creation source: web, telegram, browser_extension, wechat, or unknown."`
	OrderBy            string `json:"orderBy,omitempty" jsonschema:"create_time asc, create_time desc (default), update_time asc, or update_time desc."`
	PageSize           *int   `json:"pageSize,omitempty" jsonschema:"Results per page, from 1 to 50 (default 20)."`
	PageToken          string `json:"pageToken,omitempty" jsonschema:"Pass nextPageToken from the preceding search with the same filters and order."`
}

func registerPersonalMemoSearch(server *sdkmcp.Server, execute toolExecutor, listOperation *registeredOperation) {
	const title = "Search Memos"
	sdkmcp.AddTool(server, &sdkmcp.Tool{
		Name:         "memo_search",
		Title:        title,
		Description:  "Search your active top-level memos. All supplied filters must match. Results include the Markdown content and a nextPageToken when more results are available.",
		OutputSchema: outputSchemaForOperation(listOperation.Operation),
		Annotations:  annotationsForOperation(listOperation.Operation, title),
	}, func(ctx context.Context, _ *sdkmcp.CallToolRequest, input memoSearchInput) (*sdkmcp.CallToolResult, any, error) {
		arguments, err := input.listArguments()
		if err != nil {
			return nil, nil, err
		}
		result, err := execute(ctx, listOperation.Operation, arguments)
		return result, nil, err
	})
}

func (input memoSearchInput) listArguments() (map[string]any, error) {
	pageSize := defaultMemoSearchPageSize
	if input.PageSize != nil {
		pageSize = *input.PageSize
	}
	if pageSize < 1 || pageSize > maxMemoSearchPageSize {
		return nil, errors.New("pageSize must be between 1 and 50")
	}
	arguments := map[string]any{"pageSize": pageSize}
	if input.PageToken != "" {
		if err := validateMemoSearchPageToken(input.PageToken); err != nil {
			return nil, err
		}
		arguments["pageToken"] = input.PageToken
	}

	switch input.OrderBy {
	case "":
	case "create_time asc", "create_time desc", "update_time asc", "update_time desc":
		arguments["orderBy"] = input.OrderBy
	default:
		return nil, errors.New("orderBy must be create_time asc, create_time desc, update_time asc, or update_time desc")
	}

	filters, err := memoSearchDateFilters(input.StartDate, input.EndDate)
	if err != nil {
		return nil, err
	}
	if input.Query != "" {
		filters = append(filters, "content.contains("+strconv.Quote(input.Query)+")")
	}
	if input.Tag != "" {
		tag := strings.TrimPrefix(strings.TrimSpace(input.Tag), "#")
		if tag == "" {
			return nil, errors.New("tag must contain a tag path")
		}
		filters = append(filters, filter.TagBranchExpression(tag))
	}
	if input.HasTags != nil {
		if *input.HasTags {
			filters = append(filters, "size(tags) > 0")
		} else {
			filters = append(filters, "size(tags) == 0")
		}
	}
	if input.HasIncompleteTasks != nil {
		filters = append(filters, "has_incomplete_tasks == "+strconv.FormatBool(*input.HasIncompleteTasks))
	}
	if input.HasAttachments != nil {
		filters = append(filters, "has_attachments == "+strconv.FormatBool(*input.HasAttachments))
	}
	if input.Pinned != nil {
		filters = append(filters, "pinned == "+strconv.FormatBool(*input.Pinned))
	}
	if input.CreationChannel != "" {
		if !store.IsMemoCreationChannel(input.CreationChannel) {
			return nil, errors.New("creationChannel must be web, telegram, browser_extension, wechat, or unknown")
		}
		filters = append(filters, "creation_channel == "+strconv.Quote(input.CreationChannel))
	}
	if len(filters) > 0 {
		arguments["filter"] = strings.Join(filters, " && ")
	}
	return arguments, nil
}

func memoSearchDateFilters(startDate, endDate string) ([]string, error) {
	if startDate == "" && endDate == "" {
		return nil, nil
	}
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nil, errors.Wrap(err, "load memo search timezone")
	}
	var filters []string
	var start time.Time
	if startDate != "" {
		start, err = time.ParseInLocation(time.DateOnly, startDate, shanghai)
		if err != nil {
			return nil, errors.New("startDate must be a valid YYYY-MM-DD date")
		}
		filters = append(filters, "created_ts >= timestamp("+strconv.FormatInt(start.Unix(), 10)+")")
	}
	if endDate != "" {
		end, err := time.ParseInLocation(time.DateOnly, endDate, shanghai)
		if err != nil {
			return nil, errors.New("endDate must be a valid YYYY-MM-DD date")
		}
		if startDate != "" && end.Before(start) {
			return nil, errors.New("endDate must be on or after startDate")
		}
		filters = append(filters, "created_ts < timestamp("+strconv.FormatInt(end.AddDate(0, 0, 1).Unix(), 10)+")")
	}
	return filters, nil
}

func validateMemoSearchPageToken(value string) error {
	// ListMemos takes the limit from the previous page token when it is present.
	// Check the same token format so search's limit applies to every page.
	if len(value) > 128 {
		return errors.New("pageToken is too long")
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return errors.New("pageToken must be a nextPageToken from memo_search")
	}
	var page apipb.PageToken
	if err := proto.Unmarshal(data, &page); err != nil || page.Limit < 1 || page.Limit > maxMemoSearchPageSize || page.Offset < 0 {
		return errors.New("pageToken must be a nextPageToken from memo_search")
	}
	return nil
}
