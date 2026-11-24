package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/server/auth"
)

// personalAPI binds the five schema-derived tools directly to the note service.
type personalAPI struct {
	service *api.APIService
}

func (a personalAPI) execute(ctx context.Context, operation *openAPIOperation, arguments map[string]any) (*sdkmcp.CallToolResult, error) {
	if auth.GetUserID(ctx) == 0 {
		return personalToolError(status.Error(codes.Unauthenticated, "MCP connection required")), nil
	}
	fields := make(map[string]any)
	for _, parameter := range operation.Parameters {
		if value := arguments[parameter.Name]; parameter.In == "query" && value != nil {
			fields[parameter.Name] = value
		}
	}

	switch operation.OperationID {
	case "AuthService_GetCurrentUser":
		return invokePersonal(ctx, fields, &apipb.GetCurrentUserRequest{}, a.service.GetCurrentUser)
	case "MemoService_ListMemos":
		return invokePersonal(ctx, fields, &apipb.ListMemosRequest{}, a.service.ListMCPMemos)
	case "MemoService_GetMemo":
		fields["name"] = personalMemoName(arguments)
		return invokePersonal(ctx, fields, &apipb.GetMemoRequest{}, a.service.GetMemo)
	case "MemoService_CreateMemo":
		fields["memo"] = arguments["body"]
		return invokePersonal(ctx, fields, &apipb.CreateMemoRequest{}, a.service.CreateMemo)
	case "MemoService_UpdateMemo":
		body, ok := arguments["body"].(map[string]any)
		if !ok {
			return personalToolError(status.Error(codes.InvalidArgument, "memo body is required")), nil
		}
		if fields["updateMask"] == nil {
			// Include keys explicitly set to false, zero or an empty list.
			fields["updateMask"] = strings.Join(slices.Sorted(maps.Keys(body)), ",")
		}
		memo := maps.Clone(body)
		memo["name"] = personalMemoName(arguments)
		fields["memo"] = memo
		return invokePersonal(ctx, fields, &apipb.UpdateMemoRequest{}, a.service.UpdateMemo)
	default:
		return personalToolError(status.Error(codes.PermissionDenied, "MCP connection cannot access this procedure")), nil
	}
}

func personalMemoName(arguments map[string]any) string {
	name, ok := arguments["memo"].(string)
	if !ok || name == "" {
		return ""
	}
	return api.MemoNamePrefix + strings.TrimPrefix(name, api.MemoNamePrefix)
}

func invokePersonal[Request proto.Message, Response proto.Message](
	ctx context.Context, fields map[string]any, request Request, call func(context.Context, Request) (Response, error),
) (*sdkmcp.CallToolResult, error) {
	input, err := json.Marshal(fields)
	if err != nil {
		return personalToolError(status.Error(codes.InvalidArgument, err.Error())), nil
	}
	if err := protojson.Unmarshal(input, request); err != nil {
		return personalToolError(status.Error(codes.InvalidArgument, err.Error())), nil
	}
	response, err := call(ctx, request)
	if err != nil {
		return personalToolError(err), nil
	}
	encoded, err := (protojson.MarshalOptions{EmitDefaultValues: true}).Marshal(response)
	if err != nil {
		return nil, err
	}
	return &sdkmcp.CallToolResult{
		Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: string(encoded)}},
		StructuredContent: json.RawMessage(encoded),
	}, nil
}

func personalToolError(err error) *sdkmcp.CallToolResult {
	st := status.Convert(err)
	code := api.HTTPStatusFromCode(st.Code())
	return newToolErrorResult(fmt.Sprintf("%d %s: %s", code, http.StatusText(code), st.Message()))
}
