package mcp

import (
	"encoding/json"
	"strconv"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestCuratedOperationIDs(t *testing.T) {
	require.ElementsMatch(t, []string{
		"AuthService_GetCurrentUser", "MemoService_ListMemos", "MemoService_GetMemo",
		"MemoService_CreateMemo", "MemoService_UpdateMemo",
	}, curatedOperationIDs)
}

func TestToolNameFromOperationID(t *testing.T) {
	require.Equal(t, "memo_list_memos", toolNameFromOperationID("MemoService_ListMemos"))
	require.Equal(t, "memo_get_memo", toolNameFromOperationID("MemoService_GetMemo"))
}

func TestBuildToolFromOperationIncludesSchemasAndMetadata(t *testing.T) {
	spec, err := loadOpenAPISpec("../../proto/gen/openapi.yaml")
	require.NoError(t, err)
	registry, err := buildOperationRegistry(spec)
	require.NoError(t, err)

	tool, operation := buildToolFromOperation(registry["MemoService_ListMemos"])
	require.Equal(t, "memo_list_memos", tool.Name)
	require.Equal(t, "Memo List Memos", tool.Title)
	require.Equal(t, "MemoService_ListMemos", operation.OperationID)
	require.Equal(t, "GET", operation.Method)
	require.Equal(t, "/api/memos", operation.Path)
	require.Equal(t, "MemoService_ListMemos", tool.Meta["operationId"])
	require.Equal(t, "GET", tool.Meta["method"])
	require.Equal(t, "/api/memos", tool.Meta["path"])
	require.NotEmpty(t, tool.Description)
	require.NotNil(t, tool.InputSchema)
	require.NotNil(t, tool.OutputSchema)
	require.NotNil(t, tool.Annotations)
	require.True(t, tool.Annotations.ReadOnlyHint)
	require.False(t, *tool.Annotations.DestructiveHint)
	require.True(t, tool.Annotations.IdempotentHint)
	require.False(t, *tool.Annotations.OpenWorldHint)

	inputBytes, err := json.Marshal(tool.InputSchema)
	require.NoError(t, err)
	require.Contains(t, string(inputBytes), `"pageSize"`)
	require.Contains(t, string(inputBytes), `"additionalProperties":false`)

	outputBytes, err := json.Marshal(tool.OutputSchema)
	require.NoError(t, err)
	require.Contains(t, string(outputBytes), `"memos"`)
}

func TestBuildToolFromOperationIncludesRequestBodySchema(t *testing.T) {
	spec, err := loadOpenAPISpec("../../proto/gen/openapi.yaml")
	require.NoError(t, err)
	registry, err := buildOperationRegistry(spec)
	require.NoError(t, err)

	tool, operation := buildToolFromOperation(registry["MemoService_CreateMemo"])
	require.Equal(t, "POST", operation.Method)
	require.False(t, tool.Annotations.ReadOnlyHint)
	require.False(t, *tool.Annotations.DestructiveHint)
	require.False(t, tool.Annotations.IdempotentHint)

	input, ok := tool.InputSchema.(jsonSchema)
	require.True(t, ok)
	require.Contains(t, input["required"], "body")
	properties, ok := input["properties"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, properties, "memoId")
	require.Contains(t, properties, "body")
	body, ok := properties["body"].(jsonSchema)
	require.True(t, ok)
	require.Equal(t, "object", body["type"])
	require.Contains(t, body["properties"], "content")

	err = validateToolArguments(input, map[string]any{
		"body": map[string]any{
			"content": "hello",
		},
	})
	require.NoError(t, err)
}

func TestBuildToolFromOperationRejectsEmptyMemoUpdateBody(t *testing.T) {
	spec, err := loadOpenAPISpec("../../proto/gen/openapi.yaml")
	require.NoError(t, err)
	registry, err := buildOperationRegistry(spec)
	require.NoError(t, err)

	tool, _ := buildToolFromOperation(registry["MemoService_UpdateMemo"])
	input, ok := tool.InputSchema.(jsonSchema)
	require.True(t, ok)

	// An empty body carries no fields to update; reject it at the schema instead of
	// letting the gateway infer an empty field mask and fail late.
	require.Error(t, validateToolArguments(input, map[string]any{
		"memo": "memos/abc123",
		"body": map[string]any{},
	}))
	require.NoError(t, validateToolArguments(input, map[string]any{
		"memo": "memos/abc123",
		"body": map[string]any{"content": "updated"},
	}))
}

func TestBuildToolFromOperationExposesCurrentUser(t *testing.T) {
	spec, err := loadOpenAPISpec("../../proto/gen/openapi.yaml")
	require.NoError(t, err)
	registry, err := buildOperationRegistry(spec)
	require.NoError(t, err)

	tool, operation := buildToolFromOperation(registry["AuthService_GetCurrentUser"])
	require.Equal(t, "auth_get_current_user", tool.Name)
	require.Equal(t, "GET", operation.Method)
	require.True(t, tool.Annotations.ReadOnlyHint)
}

func TestBuildToolFromOperationMarksUpdateMemoDestructive(t *testing.T) {
	spec, err := loadOpenAPISpec("../../proto/gen/openapi.yaml")
	require.NoError(t, err)
	registry, err := buildOperationRegistry(spec)
	require.NoError(t, err)

	tool, operation := buildToolFromOperation(registry["MemoService_UpdateMemo"])
	require.Equal(t, "PATCH", operation.Method)
	require.False(t, tool.Annotations.ReadOnlyHint)
	require.True(t, *tool.Annotations.DestructiveHint)
	require.False(t, tool.Annotations.IdempotentHint)
}

func TestBuildCuratedToolsHasUniqueNames(t *testing.T) {
	spec, err := loadOpenAPISpec("../../proto/gen/openapi.yaml")
	require.NoError(t, err)
	registry, err := buildOperationRegistry(spec)
	require.NoError(t, err)

	tools, operations, err := buildCuratedTools(registry)
	require.NoError(t, err)
	require.Len(t, tools, len(curatedOperationIDs))
	require.Len(t, operations, len(curatedOperationIDs))

	names := map[string]struct{}{}
	for _, tool := range tools {
		require.IsType(t, &sdkmcp.Tool{}, tool)
		require.NotEmpty(t, tool.Name)
		require.NotContains(t, names, tool.Name)
		names[tool.Name] = struct{}{}
		require.Equal(t, tool.Name, operations[tool.Name].ToolName)

		inputBytes, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err)
		require.NotContains(t, string(inputBytes), "#/components/schemas")
		outputBytes, err := json.Marshal(tool.OutputSchema)
		require.NoError(t, err)
		require.NotContains(t, string(outputBytes), "#/components/schemas")
	}
}

func TestBuildCuratedToolsRejectsMissingOperation(t *testing.T) {
	_, _, err := buildCuratedTools(map[string]*openAPIOperation{})
	require.ErrorContains(t, err, "curated OpenAPI operation")
	require.ErrorContains(t, err, "not found")
}

func TestValidateOperationOverridesRejectsStaleKey(t *testing.T) {
	spec, err := loadOpenAPISpec("../../proto/gen/openapi.yaml")
	require.NoError(t, err)
	registry, err := buildOperationRegistry(spec)
	require.NoError(t, err)
	require.NoError(t, validateOperationOverrides(registry))

	// A renamed/removed operation must be reported instead of silently losing its
	// override.
	delete(registry, "MemoService_UpdateMemo")
	require.ErrorContains(t, validateOperationOverrides(registry), "MemoService_UpdateMemo")
}

func TestBuildCuratedToolsRejectsDuplicateToolNames(t *testing.T) {
	registry := make(map[string]*openAPIOperation, len(curatedOperationIDs))
	for _, operationID := range curatedOperationIDs {
		registry[operationID] = &openAPIOperation{
			OperationID:    operationID,
			Description:    operationID,
			Method:         "GET",
			Path:           "/api/test",
			ResponseSchema: okSchema(),
		}
	}
	registry["MemoService_ListMemos"].OperationID = "MemoService_GetMemo"

	_, _, err := buildCuratedTools(registry)
	require.ErrorContains(t, err, "duplicate MCP tool name")
}

func TestBuildCuratedToolsUseStandardSchemaFormats(t *testing.T) {
	spec, err := loadOpenAPISpec("../../proto/gen/openapi.yaml")
	require.NoError(t, err)
	registry, err := buildOperationRegistry(spec)
	require.NoError(t, err)

	tools, _, err := buildCuratedTools(registry)
	require.NoError(t, err)

	// Formats registered by JSON Schema or ajv-formats; anything else trips
	// strict clients (see https://github.com/usememos/memos/issues/6262).
	allowedFormats := map[string]struct{}{
		"date-time": {},
		"int32":     {},
		"int64":     {},
		"float":     {},
		"double":    {},
	}
	var collectFormats func(t *testing.T, tool string, path string, value any)
	collectFormats = func(t *testing.T, tool string, path string, value any) {
		switch typed := value.(type) {
		case map[string]any:
			if format, ok := typed["format"]; ok {
				formatName, isString := format.(string)
				require.True(t, isString, "%s: %s has non-string format %v", tool, path, format)
				require.Contains(t, allowedFormats, formatName, "%s: %s uses non-standard format %q", tool, path, formatName)
			}
			for key, item := range typed {
				collectFormats(t, tool, path+"/"+key, item)
			}
		case []any:
			for index, item := range typed {
				collectFormats(t, tool, path+"/"+strconv.Itoa(index), item)
			}
		default:
			// Scalars carry no nested schemas.
		}
	}

	for _, tool := range tools {
		for label, schema := range map[string]any{"inputSchema": tool.InputSchema, "outputSchema": tool.OutputSchema} {
			encoded, err := json.Marshal(schema)
			require.NoError(t, err)
			var decoded any
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			collectFormats(t, tool.Name, label, decoded)
		}
	}
}
