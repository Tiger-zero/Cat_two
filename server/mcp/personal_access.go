package mcp

import (
	"context"
	"slices"
	"strconv"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usememos/memos/internal/ratelimit"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

type connectionPermissionContextKey struct{}

// Keep the read-only surface explicit. A new tool is write-capable for
// authorization purposes until it has been reviewed and added here.
func isReadOnlyPersonalTool(name string) bool {
	switch name {
	case "auth_get_current_user", "memo_list_memos", "memo_get_memo", "memo_search", "memo_batch_get", "tag_tree", "tag_search":
		return true
	default:
		return false
	}
}

func (s *MCPService) authorizePersonalTools(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
	return func(ctx context.Context, method string, request sdkmcp.Request) (sdkmcp.Result, error) {
		permission, authenticated := ctx.Value(connectionPermissionContextKey{}).(store.MCPConnectionPermission)
		if call, ok := request.(*sdkmcp.CallToolRequest); ok {
			userID := auth.GetUserID(ctx)
			if !authenticated || userID <= 0 || (permission != store.MCPConnectionReadOnly && permission != store.MCPConnectionReadWrite) {
				return newToolErrorResult("MCP connection authentication required"), nil
			}
			if s.RateLimiter != nil && !s.RateLimiter.Consume(ratelimit.ScopeAuthenticated, strconv.FormatInt(int64(userID), 10), 1).Allowed {
				return newToolErrorResult("MCP request rate limit exceeded"), nil
			}
			if permission == store.MCPConnectionReadOnly && (call.Params == nil || !isReadOnlyPersonalTool(call.Params.Name)) {
				return newToolErrorResult("this MCP connection has read-only access"), nil
			}
		}

		result, err := next(ctx, method, request)
		if err != nil {
			return result, err
		}
		switch typed := result.(type) {
		case *sdkmcp.ListToolsResult:
			// Two clients of the same instance can have different permissions.
			typed.CacheScope = "private"
			if permission == store.MCPConnectionReadOnly {
				typed.Tools = slices.DeleteFunc(slices.Clone(typed.Tools), func(tool *sdkmcp.Tool) bool { return !isReadOnlyPersonalTool(tool.Name) })
			}
		case *sdkmcp.DiscoverResult:
			typed.CacheScope = "private"
			if permission == store.MCPConnectionReadOnly {
				typed.Instructions = "This connection has read-only access. Use tools/list to discover available tools."
			}
		default:
			// Other results have no permission-dependent metadata.
		}
		return result, nil
	}
}
