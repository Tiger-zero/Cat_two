package mcp

import (
	"slices"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestPersonalMemoBatchEnforcesIdentityAndInputSchema(t *testing.T) {
	ctx := t.Context()
	server := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "server", Version: "test"}, nil)
	// A direct tool invocation without identity must fail before querying the store.
	registerPersonalMemoBatchGet(server, nil)
	serverTransport, clientTransport := sdkmcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, serverSession.Close()) })
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })

	valid := []string{"memos/valid"}
	for _, tc := range []struct {
		name    string
		names   []string
		message string
	}{
		{name: "missing identity", names: valid, message: "MCP authentication required"},
		{name: "empty list", names: []string{}, message: `validating "arguments"`},
		{name: "too many names before deduplication", names: slices.Repeat(valid, 51), message: `validating "arguments"`},
		{name: "mixed canonical and bare name", names: []string{"memos/valid", "bare"}, message: `validating "arguments"`},
		{name: "nested name", names: []string{"memos/valid/extra"}, message: `validating "arguments"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
				Name:      "memo_batch_get",
				Arguments: map[string]any{"names": tc.names},
			})
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.Nil(t, result.StructuredContent)
			require.Len(t, result.Content, 1)
			text, ok := result.Content[0].(*sdkmcp.TextContent)
			require.True(t, ok)
			require.Contains(t, text.Text, tc.message)
		})
	}
}
