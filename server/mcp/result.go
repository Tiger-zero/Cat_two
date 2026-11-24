package mcp

import sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

// newToolErrorResult reports a tool execution error through IsError and text
// content only. Error results must not carry structuredContent: every tool
// declares an outputSchema describing its success payload, and spec-strict
// clients validate structuredContent against that schema — an {"error": ...}
// object fails validation and masks the real error message.
func newToolErrorResult(message string) *sdkmcp.CallToolResult {
	return &sdkmcp.CallToolResult{
		Content: []sdkmcp.Content{
			&sdkmcp.TextContent{Text: message},
		},
		IsError: true,
	}
}
