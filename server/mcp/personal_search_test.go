package mcp

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/usememos/memos/filter"
	apipb "github.com/usememos/memos/proto/gen/api"
)

func TestMemoSearchCompilesLiteralFilters(t *testing.T) {
	t.Parallel()

	input := memoSearchInput{
		Query:              "进度 50%_off \"quote\" \\ notes\nnext",
		Tag:                "#计划/团队",
		StartDate:          "2024-02-29",
		EndDate:            "2024-02-29",
		HasIncompleteTasks: new(false),
		HasAttachments:     new(true),
		Pinned:             new(false),
		CreationChannel:    "web",
		OrderBy:            "update_time asc",
	}
	args, err := input.listArguments()
	require.NoError(t, err)
	require.Equal(t, defaultMemoSearchPageSize, args["pageSize"])
	require.Equal(t, input.OrderBy, args["orderBy"])
	expression, ok := args["filter"].(string)
	require.True(t, ok)
	require.Contains(t, expression, "has_incomplete_tasks == false")
	require.Contains(t, expression, "has_attachments == true")
	require.Contains(t, expression, "pinned == false")

	engine, err := filter.DefaultEngine()
	require.NoError(t, err)
	statement, err := engine.CompileToStatement(t.Context(), expression)
	require.NoError(t, err)
	require.Contains(t, statement.Args, "计划/团队")
	require.Contains(t, statement.Args, "web")
	// Shanghai midnight is 16:00 UTC on the preceding civil date.
	require.Contains(t, statement.Args, time.Date(2024, time.February, 28, 16, 0, 0, 0, time.UTC).Unix())
	require.Contains(t, statement.Args, time.Date(2024, time.February, 29, 16, 0, 0, 0, time.UTC).Unix())
	require.Contains(t, statement.Args, "%进度 50\\%\\_off \"quote\" \\\\ notes\nnext%")
	require.Contains(t, statement.Args, "计划/团队/")
	require.NotContains(t, statement.SQL, input.Query)
}

func TestMemoSearchValidatesFiltersAndPageTokens(t *testing.T) {
	t.Parallel()

	token := func(size int32) string {
		raw, err := proto.Marshal(&apipb.PageToken{Limit: size, Offset: 10})
		require.NoError(t, err)
		return base64.StdEncoding.EncodeToString(raw)
	}
	args, err := (memoSearchInput{PageToken: token(50)}).listArguments()
	require.NoError(t, err)
	require.Equal(t, token(50), args["pageToken"])
	require.NotContains(t, args, "filter")

	for _, tt := range []struct {
		input memoSearchInput
		field string
	}{
		{input: memoSearchInput{StartDate: "2026-02-29"}, field: "startDate"},
		{input: memoSearchInput{EndDate: "not-a-date"}, field: "endDate"},
		{input: memoSearchInput{StartDate: "2026-01-02", EndDate: "2026-01-01"}, field: "endDate"},
		{input: memoSearchInput{PageSize: new(0)}, field: "pageSize"},
		{input: memoSearchInput{PageSize: new(51)}, field: "pageSize"},
		{input: memoSearchInput{PageToken: token(51)}, field: "pageToken"},
		{input: memoSearchInput{PageToken: "not-base64!"}, field: "pageToken"},
		{input: memoSearchInput{PageToken: strings.Repeat("a", 129)}, field: "pageToken"},
		{input: memoSearchInput{CreationChannel: "unsupported"}, field: "creationChannel"},
		{input: memoSearchInput{OrderBy: "pinned desc"}, field: "orderBy"},
		{input: memoSearchInput{Tag: "#"}, field: "tag"},
	} {
		_, err := tt.input.listArguments()
		require.ErrorContains(t, err, tt.field)
	}
}
