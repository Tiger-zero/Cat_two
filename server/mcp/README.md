# Cat_two MCP server

Cat_two serves `/mcp` over stateless Streamable HTTP with JSON responses. The tools use the same typed service methods and author permissions as the Cat_two web app. Clients use a dedicated MCP connection token for every request. File uploads and downloads are available in the signed-in web app.

## Transport and authentication

- Keep the JSON request body within 16 MiB. Desktop clients can omit `Origin`. Browser origins must match the configured instance URL or a direct connection. Set `MEMOS_INSTANCE_URL` to the public HTTPS URL when using a reverse proxy.
- Missing or invalid tokens return `401`. Inactive or expired connections return `403`. Read-only connections discover and call only read tools. The API checks the owner and current permissions on every request, even when a client caches its tool catalog.
- The first authenticated POST records its time and client IP. Tool calls share the authenticated API rate limit. The transport applies a per-IP rate limit before token lookup.
- The SDK negotiates earlier protocols through `initialize`. Clients using `2026-07-28` call `server/discover` and send per-request protocol metadata and the `Mcp-Protocol-Version`, `Mcp-Method` and relevant `Mcp-Name` headers.
- Discovery and tool list responses are private and advertise a 24-hour cache lifetime. The server exposes tools as its capability.

### Connecting a client

Point a Streamable HTTP client at `https://<your-instance>/mcp`. Sign in
to Cat_two, open **Settings → MCP**, and create
an MCP connection. Choose 7 days (the default), 14 days, one calendar year
from issuance, or a permanent token. Choose read-only or read and write.
Read and write is the default.
The token works immediately and is shown once at creation.
In Codex, leave the Bearer token environment-variable field blank and set the
`Authorization` request header to the full copied value. The following JSON
illustrates the equivalent configuration for clients that use this format.

```json
{
  "mcpServers": {
    "cat_two": {
      "type": "http",
      "url": "https://<your-instance>/mcp",
      "headers": {
        "Authorization": "Bearer <your-mcp-connection-token>"
      }
    }
  }
}
```

Restart the client if it prompts after saving the connection. Settings shows
the first connection's client IP and the token expiration. The connection
expires when its token does. Deleting the connection invalidates the token
for the next request, including from a client that has already connected.

To switch to a new instance, create a new MCP connection there and update the client URL and header. [Codex setup and troubleshooting](../../plugins/cat-two-notes/README.md#连接-nas)

### Managing Cat_two connections

The web settings page manages connections through `/internal/mcpConnections`. Management uses a signed-in web session. The browser lists the token's state, permission, expiration and first recorded client IP. Creating a connection returns the token once. Deleting it invalidates the next request.

## Tool surface

Cat_two provides `auth_get_current_user`, `memo_list_memos`, `memo_get_memo`,
`memo_create_memo`, and `memo_update_memo`, plus the search, batch and tag tools below.

The following read-only tools use the connection owner's notes.

| Tool | Parameters and scope | Result |
| --- | --- | --- |
| `memo_search` | Search active top-level notes. All supplied filters must match. `query` matches a literal substring of the Markdown body. `tag` accepts a path with an optional `#` and includes descendants. Use `hasTags: false` for untagged notes or `hasTags: true` for notes with tags. `startDate` and `endDate` are inclusive creation dates in `YYYY-MM-DD` and use Asia/Shanghai. Either bound can be omitted. `hasIncompleteTasks`, `hasAttachments`, and `pinned` accept booleans. `creationChannel` accepts `web`, `telegram`, `browser_extension`, `wechat`, or `unknown`. | `{memos, nextPageToken?}`. The memos include Markdown content. Use `pageSize` between 1 and 50 (default 20) and pass `nextPageToken` as `pageToken` with the same search parameters. `orderBy` accepts `create_time asc`, `create_time desc` (default), `update_time asc` or `update_time desc`. |
| `memo_batch_get` | `names` contains 1–50 canonical `memos/<uid>` names. The first occurrence of each name determines the order. Reads owned active and archived notes. | `{memos, unavailable}`. Memos include `name`, Markdown `content`, `tags`, `createTime`, `updateTime`, `pinned`, `state`, `creationChannel`, and `revision`. Timestamps are RFC 3339 in UTC. Missing, foreign and trashed notes go into `unavailable` by name. Batches above 4 MiB of Markdown return a tool error. Split larger batches or use `memo_get_memo` for a single note. |
| `tag_tree` | Browse tag paths in active top-level notes. Optional `offset` starts at 0 and `pageSize` defaults to 50 with a maximum of 200. | `{tags: [{path, parent?, memoCount}], total, nextOffset?}`. Results are sorted by full path. Counts include descendants and count each note once per path. |
| `tag_search` | Search the same tag paths by a nonblank, case-insensitive substring `query` with an optional leading `#`. Uses the same pagination as `tag_tree`. | The same result shape as `tag_tree`. `total` counts matching paths before pagination. |

For example, call `memo_search` with
`{"tag":"project/cat_two","startDate":"2026-11-01","endDate":"2026-11-20","hasIncompleteTasks":true}`.
For a CEL filter expression or archived search, use `memo_list_memos`.
Both tools return up to 4 MiB of Markdown per page. Follow `nextPageToken`
when a page stops short of `pageSize`. If a single memo exceeds the budget,
the tool error names it for `memo_get_memo` and gives a resume token when
more results remain. Retry an interrupted page with its original token.
Clients connected to older images can also use `memo_list_memos` and
`memo_get_memo` when the new tools are absent from discovery.

Write Markdown tags such as `#project/cat_two` in `body.content` of
`memo_create_memo` or `memo_update_memo`. Creation accepts an optional
`body.createTime` in RFC 3339. For safe retry, set top-level `memoId` and
`idempotent: true` and preserve exactly the same inputs. A matching replay
returns the current readable note. Reusing the ID for a different payload
returns a conflict. For an update to the body, use `updateMask: "content"`
and pass the revision returned by the read as top-level `expectedRevision`.
If the revision changed, read the latest note and review the proposed change
before retrying.

## Request flow

Startup resolves request and response schemas only for the five published operations selected in `catalog.go`. The embedded OpenAPI generated from Proto HTTP annotations supplies these MCP tool schemas. MCP tools compile references to local JSON Schema definitions. The Cat_two search, batch and tag tools are registered with their own typed inputs.

`service.go` validates the connection and tool arguments. `personal_api.go` binds requests directly to the API service, applies the same partial-update mask semantics as browser Connect, and returns typed results in `structuredContent`. Tool errors set `isError` and use a text block.

## Verification

Regenerate API schemas from the corresponding Proto sources after changing an API contract. Update the MCP catalog, typed handler and permission list together when adding a tool. Run the registration, schema, permission and API integration checks.

```bash
go test ./server/mcp/...
go test ./server/test -run 'TestPersonalMCP|TestIntegrationClientsUseSameNoteOwnership'
```
