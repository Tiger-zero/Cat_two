# Historical Memos MCP evaluation

[memos_eval.xml](memos_eval.xml) preserves 15 task-level questions and expected answers from the upstream Memos MCP evaluation. The answers describe the multi-account demo seed used by that evaluation. Several questions depend on Spaces, invitations, comments tools, or the upstream personal access token transport.

## Current Cat_two verification

Cat_two uses a dedicated MCP connection token and an owner-scoped tool catalog. Verify the deployed tool surface with `tools/list`. For an end-to-end exercise, use an isolated SQLite instance with your own fixture notes and a read-only connection. Search for a keyword, check tag counts, and batch-read the known resource names. Compare with the notes seeded in that instance. [Connection instructions](../../../plugins/cat-two-notes/README.md#连接-nas) · [Tool and permission reference](../../../.agents/skills/cat-two-product-development/references/external-api.md#mcp-查找与共同创作)

From the repository root, run the Cat_two MCP and server tests.

```sh
go test ./server/mcp/... ./server/test/...
```
