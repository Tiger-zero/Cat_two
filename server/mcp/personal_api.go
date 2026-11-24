package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/grpc-ecosystem/grpc-gateway/v2/utilities"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/server/auth"
)

// personalAPI binds the public tool arguments to the same typed requests used
// by REST and Connect. Only authenticated MCP routes install the user identity.
type personalAPI struct {
	service   *api.APIService
	marshaler runtime.Marshaler
}

func (a personalAPI) execute(ctx context.Context, operation *openAPIOperation, arguments map[string]any) (*sdkmcp.CallToolResult, error) {
	if auth.GetUserID(ctx) == 0 {
		return personalToolError(status.Error(codes.Unauthenticated, "MCP connection required")), nil
	}
	switch operation.OperationID {
	case "AuthService_GetCurrentUser":
		return invokePersonal(ctx, a, operation, arguments, &apipb.GetCurrentUserRequest{}, a.service.GetCurrentUser)
	case "MemoService_ListMemos":
		return invokePersonal(ctx, a, operation, arguments, &apipb.ListMemosRequest{}, a.service.ListMCPMemos)
	case "MemoService_GetMemo":
		request := &apipb.GetMemoRequest{Name: personalMemoName(arguments)}
		return invokePersonal(ctx, a, operation, arguments, request, a.service.GetMemo)
	case "MemoService_CreateMemo":
		memo, _, err := a.memoBody(arguments)
		if err != nil {
			return personalToolError(err), nil
		}
		return invokePersonal(ctx, a, operation, arguments, &apipb.CreateMemoRequest{Memo: memo}, a.service.CreateMemo)
	case "MemoService_UpdateMemo":
		memo, body, err := a.memoBody(arguments)
		if err != nil {
			return personalToolError(err), nil
		}
		memo.Name = personalMemoName(arguments)
		mask, err := runtime.FieldMaskFromRequestBody(bytes.NewReader(body), memo)
		if err != nil {
			return personalToolError(status.Error(codes.InvalidArgument, err.Error())), nil
		}
		request := &apipb.UpdateMemoRequest{Memo: memo, UpdateMask: mask}
		return invokePersonal(ctx, a, operation, arguments, request, a.service.UpdateMemo)
	default:
		return personalToolError(status.Error(codes.PermissionDenied, "MCP connection cannot access this API procedure")), nil
	}
}

func (a personalAPI) memoBody(arguments map[string]any) (*apipb.Memo, []byte, error) {
	body, err := json.Marshal(arguments["body"])
	if err != nil {
		return nil, nil, status.Error(codes.InvalidArgument, err.Error())
	}
	memo := &apipb.Memo{}
	if err := a.marshaler.Unmarshal(body, memo); err != nil {
		return nil, nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return memo, body, nil
}

func personalMemoName(arguments map[string]any) string {
	name := valueToString(arguments["memo"])
	if id, ok := strings.CutPrefix(name, api.MemoNamePrefix); ok && id != "" && !strings.Contains(id, "/") {
		return name
	}
	return api.MemoNamePrefix + name
}

func invokePersonal[Request proto.Message, Response proto.Message](
	ctx context.Context, a personalAPI, operation *openAPIOperation, arguments map[string]any,
	request Request, call func(context.Context, Request) (Response, error),
) (*sdkmcp.CallToolResult, error) {
	if err := runtime.PopulateQueryParameters(request, operationQuery(operation, arguments), &utilities.DoubleArray{}); err != nil {
		return personalToolError(status.Error(codes.InvalidArgument, err.Error())), nil
	}
	response, err := call(ctx, request)
	if err != nil {
		return personalToolError(err), nil
	}
	encoded, err := a.marshaler.Marshal(response)
	if err != nil {
		return nil, err
	}
	return &sdkmcp.CallToolResult{
		Content:           []sdkmcp.Content{&sdkmcp.TextContent{Text: string(encoded)}},
		StructuredContent: json.RawMessage(encoded),
	}, nil
}

func personalToolError(err error) *sdkmcp.CallToolResult {
	grpcStatus := status.Convert(err)
	return newToolErrorResult(apiErrorMessage(runtime.HTTPStatusFromCode(grpcStatus.Code()), map[string]any{"message": grpcStatus.Message()}))
}
