# MCP Server

This package serves an [OpenAPI](https://www.openapis.org/)-driven
[Model Context Protocol](https://modelcontextprotocol.io/) (MCP) endpoint at
`/mcp`. It exposes a curated, memo-focused toolset over the **Streamable HTTP**
transport using the official `github.com/modelcontextprotocol/go-sdk`.

Tool calls execute in-process against the existing REST API. Each tool comes
from an operation in the generated OpenAPI document (`proto/gen/openapi.yaml`,
embedded via `proto.OpenAPIYAML()`). The tool handler translates each call into
the matching `/api/...` request so API authorization applies. In Cat_two
personal mode, the transport authenticates a dedicated MCP connection token
and passes the connection owner's identity to the API internally.

## Integration

`server.NewServer` calls `mcp.NewMCPService` after registering the API, file, and gRPC-gateway routes, passing the same HTTP handler:

```go
mcpService, err := mcp.NewMCPService(profile, s.handler, s.Store)
if err != nil {
    return nil, errors.Wrap(err, "failed to create MCP service")
}
mcpService.RegisterRoutes(mux)
```

The service advertises the **tools** capability only — no prompts, no resources.

## Startup flow

`NewMCPService` (`service.go`) wires everything up at construction time and fails
fast on any inconsistency:

1. `loadMCPServiceOpenAPISpec` parses the embedded `proto.OpenAPIYAML()` bytes
   into an `openAPISpec`.
2. `buildOperationRegistry` (`openapi.go`) indexes every operation by
   `operationId`, recording method, path, resolved request-body schema, and
   resolved 200 response schema.
3. `buildCuratedTools` (`catalog.go`) selects the allowlisted operation IDs and
   converts each into an `*sdkmcp.Tool` plus a `registeredOperation`. Missing
   IDs or duplicate tool names are construction errors.
4. Each tool allowed in the active mode is registered with
   `server.AddTool(tool, newMCPToolHandler(...))`.
5. `sdkmcp.NewStreamableHTTPHandler` wraps the server in stateless,
   JSON-response mode (no SSE, no session tracking). Stateless mode is also
   what lets the SDK serve protocol version `2026-07-28`; older clients still
   negotiate `2025-11-25` and earlier through the legacy `initialize` handshake.
   The transport body limit is set to the API-wide limit so attachment uploads
   are not cut off by the SDK's 4 MiB default.

## Request flow

`RegisterRoutes` binds `mux.HandleFunc("/mcp", ...)`. Each request:

1. `isAllowedMCPOrigin` (`origin.go`) rejects disallowed cross-origin browser requests with `403`.
2. The request body is capped at 256 MiB before the SDK reads it.
3. In personal mode, the transport looks up the connection by its indexed
   token hash and reads the owner's current account status in the same
   query. It checks the connection status, expiration, and account status
   before SDK dispatch. The first authenticated POST records the time and
   client address. The transport passes the owner ID through the in-process
   context and strips the external token.
4. The SDK streamable handler dispatches the MCP message.
5. On a `tools/call` request, `newMCPToolHandler` (`service.go`) decodes the JSON
   arguments into a map.
6. `validateToolArguments` (`validation.go`) checks them against the tool's
   input schema. The tool handler then binds the personal-mode identity to
   the specific API procedure in the request context.
7. `apiAdapter.execute` (`adapter.go`) builds the API request
   (`buildAPIRequest`: path-parameter substitution, query encoding, JSON body),
   presents the caller's resolved client address as the request peer, and runs
   it against the HTTP handler through an `httptest.ResponseRecorder`. In
   non-personal mode it forwards the caller's Bearer header.
8. The recorder body is decoded; a non-2xx status becomes a tool error
   (`newToolErrorResult`), otherwise the value is wrapped by
   `newStructuredToolResult`.

## Schema resolution

MCP tool schemas must be self-contained JSON Schema, but the OpenAPI components
use `$ref`. `openapi.go` resolves these into local definitions:

- **Top-level inlining.** The request-body and 200-response schemas for an
  operation are resolved with `inlineRef = true`, so the outermost `$ref` is
  expanded in place (`resolveSchemaRef` → `resolveSchemaValue` → `resolveSchemaMap`).
- **Nested refs become `$defs`.** Any `$ref` encountered below the top level is
  rewritten to a local `#/$defs/<Name>` pointer, and the referenced component is
  collected into a `$defs` map (`addSchemaDef`).
- **Cycle safety.** Recursive component schemas are handled by seeding
  `defs[name]` with a placeholder and tracking `resolving[name]` before
  recursing, so a schema that references itself terminates
  (`addSchemaDef`).

`catalog.go` then assembles the per-tool input schema in
`inputSchemaForOperation`:

- Path and query parameters become top-level properties; any required
  parameter stays in `required`.
- A request body becomes a single `body` property; a required body adds `body`
  to `required`. Body `$defs` are lifted to the schema's top-level `$defs`.
- Per-operation overrides relax resource-level requirements for create and
  partial-update bodies and remove fields already supplied by a path binding
  from `body: "*"` schemas. Memo and space updates may omit `updateMask` so the
  REST gateway can infer it from the fields present in the request body. A
  `body: "*"` binding whose only field is path-bound (accepting or declining a
  space invitation) has its `body` made optional instead of demanding `{}`.
- The schema sets `"additionalProperties": false`.

The output schema is the operation's 200 `application/json` schema. When a 200
response has no JSON body, the fallback is:

```json
{ "type": "object", "properties": { "ok": { "type": "boolean" } } }
```

## Endpoint, transport & auth

- **Endpoint:** `POST /mcp` (the SDK may also use `GET`/`DELETE` on the same
  path for the Streamable HTTP transport).
- **Transport:** Streamable HTTP, **stateless**, JSON responses.
- **Protocol versions:** `2026-07-28` down to `2024-11-05`. Clients on
  `2026-07-28` skip `initialize`, call `server/discover`, carry
  `_meta.io.modelcontextprotocol/protocolVersion` on every request, and must
  send the `Mcp-Protocol-Version`, `Mcp-Method`, and (for `tools/call`)
  `Mcp-Name` headers.
- **Capabilities:** tools only, without `listChanged` — the catalog is fixed at
  startup, so `tools/list` and `server/discover` advertise a 24-hour `ttlMs`
  instead of change notifications.
- **Request size:** request bodies are limited to 256 MiB before SDK dispatch.
- **Cat_two personal mode** requires an active MCP connection token on every
  `/mcp` request, including requests from clients with a cached tool catalog.
  Missing or invalid credentials, including tokens for deleted connections,
  return `401`. Inactive connections, including old awaiting or pending
  connections and denied, revoked or expired tokens, return `403`.
- **Memos non-personal mode:** the caller's Bearer header is forwarded to the
  API. Mutating tools require a valid personal access token or access token.
  Anonymous reads follow the instance's REST API permissions.
- **Client address:** the `/mcp` request passes through the same client-address
  middleware as every other route, which resolves the caller behind trusted
  proxies. The adapter reads that resolved address from the tool handler's
  context and sets it as the in-process request's peer, so the middleware's
  second pass resolves to the same value. Without this every tool call would
  present `httptest`'s placeholder peer (`192.0.2.1`), and all anonymous MCP
  callers would share one rate-limit bucket.
- **Origin safety:** `isAllowedMCPOrigin` allows a request when the `Origin`
  header is absent (desktop clients commonly omit it), when its host matches
  the request `Host` header (host comparison only — scheme is not checked), or
  when it matches the configured `profile.InstanceURL`. Anything else gets
  `403`. This guards against DNS-rebinding from browsers.

### Connecting a client

Point a Streamable HTTP client at `https://<your-instance>/mcp`. In Cat_two
personal mode, sign in to Cat_two, open **Settings → MCP and API**, and create
an MCP connection. Choose 7 days (the default), 14 days, one calendar year
from issuance, or a permanent token. The token works immediately and is
shown once at creation.
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

After upgrading, existing active tokens retain their original expiration.
Connections approved under the old workflow retain the first request's
recorded address. Replace expired tokens or old awaiting or pending tokens by
creating a new connection. REST integrations continue to use personal access
tokens, as do MCP clients in non-personal Memos mode.
[Codex setup and troubleshooting](../../plugins/cat-two-notes/README.md#连接-nas)

### Managing Cat_two connections

Management requires a signed-in session. `GET /api/mcpConnections` lists the
account's connections with their current state, token expiration, and first
connection IP once recorded. `POST /api/mcpConnections` creates a connection
from `{"label":"My Codex","duration":"7d"}`. The duration starts at issuance.
Supported values are `7d` (the default), `14d`, `1y` (one calendar year in UTC),
and `permanent`. The creation response includes the token once.

`DELETE /api/mcpConnections/{id}` removes a connection owned by the signed-in
account and returns `204 No Content` on success. Unknown IDs and connections
owned by another account return `404`. The next request with the deleted token
fails authentication. The legacy
`POST /api/mcpConnections/{id}/decision` endpoint continues to accept
`{"action":"revoke"}`. Legacy approval and denial actions return `400`.

## Tool surface

Cat_two personal mode exposes six tools. These read the current account, list
saved views, list or read notes, and create or update notes:
`auth_get_current_user`, `user_list_memo_views`, `memo_list_memos`,
`memo_get_memo`, `memo_create_memo` and `memo_update_memo`.

The full Memos non-personal allowlist (`curatedOperationIDs` in `catalog.go`)
includes the following operations.

| OpenAPI operation | MCP tool |
| --- | --- |
| `MemoService_ListMemos` | `memo_list_memos` |
| `MemoService_CreateMemo` | `memo_create_memo` |
| `MemoService_GetMemo` | `memo_get_memo` |
| `MemoService_UpdateMemo` | `memo_update_memo` |
| `MemoService_DeleteMemo` | `memo_delete_memo` |
| `MemoService_ListMemoComments` | `memo_list_memo_comments` |
| `MemoService_CreateMemoComment` | `memo_create_memo_comment` |
| `MemoService_ListMemoAttachments` | `memo_list_memo_attachments` |
| `MemoService_SetMemoAttachments` | `memo_set_memo_attachments` |
| `MemoService_ListMemoReactions` | `memo_list_memo_reactions` |
| `MemoService_UpsertMemoReaction` | `memo_upsert_memo_reaction` |
| `MemoService_DeleteMemoReaction` | `memo_delete_memo_reaction` |
| `MemoService_ListMemoRelations` | `memo_list_memo_relations` |
| `MemoService_SetMemoRelations` | `memo_set_memo_relations` |
| `AttachmentService_ListAttachments` | `attachment_list_attachments` |
| `AttachmentService_CreateAttachment` | `attachment_create_attachment` |
| `AttachmentService_GetAttachment` | `attachment_get_attachment` |
| `AttachmentService_DeleteAttachment` | `attachment_delete_attachment` |
| `UserService_ListMemoViews` | `user_list_memo_views` |
| `AuthService_GetCurrentUser` | `auth_get_current_user` |
| `SpaceService_ListSpaces` | `space_list_spaces` |
| `SpaceService_GetSpace` | `space_get_space` |
| `SpaceService_CreateSpace` | `space_create_space` |
| `SpaceService_UpdateSpace` | `space_update_space` |
| `SpaceService_DeleteSpace` | `space_delete_space` |
| `SpaceService_ListSpaceMembers` | `space_list_space_members` |
| `SpaceService_GetSpaceMember` | `space_get_space_member` |
| `SpaceService_UpdateSpaceMember` | `space_update_space_member` |
| `SpaceService_DeleteSpaceMember` | `space_delete_space_member` |
| `SpaceService_ListSpaceInvitations` | `space_list_space_invitations` |
| `SpaceService_ListUserSpaceInvitations` | `space_list_user_space_invitations` |
| `SpaceService_GetSpaceInvitation` | `space_get_space_invitation` |
| `SpaceService_CreateSpaceInvitation` | `space_create_space_invitation` |
| `SpaceService_AcceptSpaceInvitation` | `space_accept_space_invitation` |
| `SpaceService_DeclineSpaceInvitation` | `space_decline_space_invitation` |
| `SpaceService_DeleteSpaceInvitation` | `space_delete_space_invitation` |

**Naming rule** (`toolNameFromOperationID`): drop the `Service` suffix from the
subject and convert both subject and method from camelCase to snake_case, joined
by `_`. So `MemoService_ListMemos → memo_list_memos`.

**Annotations** (`annotationsForOperation`) start from the HTTP method:

| Method | ReadOnly | Destructive | Idempotent |
| --- | --- | --- | --- |
| GET | true | false | true |
| DELETE | false | true | true |
| other (POST, PATCH, …) | false | false | false |

Per-operation overrides then correct cases the method heuristic gets wrong.
`MemoService_SetMemoAttachments` and `MemoService_SetMemoRelations` are PATCH
but declaratively replace the full set on a memo, so they report both
`IdempotentHint: true` and `DestructiveHint: true`. `MemoService_UpdateMemo`,
`SpaceService_UpdateSpace`, and `SpaceService_UpdateSpaceMember` also report
`DestructiveHint: true` because they overwrite existing fields (a member's role
included). `SpaceService_DeleteSpace` is destructive by method, and the API
also deletes every memo placed in the space.

`OpenWorldHint` is `false` for all tools. Annotations are client hints; they do
not replace API authorization.

**Result shape.** Every successful result carries object-shaped `structuredContent`
(`normalizeStructuredContent` in `result.go`):

- a JSON object is returned unchanged;
- an empty response becomes `{ "ok": true }`;
- a bare array becomes `{ "result": [...] }`;
- a scalar becomes `{ "result": value }`.

This is deliberate: it fixes [#6022](https://github.com/usememos/memos/issues/6022),
where collection tools returned a bare array that strict MCP clients reject.

Inside that envelope the API's JSON is passed through verbatim, so the gateway's
own encoding is part of the tool contract: whatever it emits is validated against
the output schema resolved from the same OpenAPI spec. grpc-gateway's stock
marshaler emits `null` for unset message fields, which no schema declares as
nullable — `RegisterGateway` therefore installs a marshaler that omits them
(`newGatewayMarshaler` in `server/api/api.go`). That fixes
[#6139](https://github.com/usememos/memos/issues/6139), where `"motionMedia": null`
failed every tool call returning an attachment.

## Error handling

Once the transport accepts a request, failures in a tool call return an MCP
`CallToolResult` with `IsError: true` and a text content block. Error results
omit `structuredContent` so strict clients use the tool's success schema only
for successful responses:

| Failure | Result |
| --- | --- |
| Arguments are not valid JSON | tool error: decode message |
| Arguments fail schema validation | tool error: validation message |
| Missing required path parameter | tool error: `missing required path parameter "..."` |
| Missing required request body | tool error: `missing required request body "body"` |
| API responds non-2xx | tool error: `"<code> <reason phrase>: <api message>"` (e.g. `"404 Not Found: ..."`) (`apiErrorMessage`) |
| API response body is not decodable JSON | tool error: decode message |

## Core files

| File | Responsibility |
| --- | --- |
| `service.go` | Constructs the MCP server, registers tools, builds the streamable HTTP handler, and binds the `/mcp` route. |
| `catalog.go` | The curated operation allowlist, tool naming, input/output schema assembly, and method-derived annotations. |
| `adapter.go` | Translates a tool call into an `/api/...` request and runs it in-process against the HTTP handler. |
| `openapi.go` | Parses the OpenAPI spec, builds the operation registry, and resolves `$ref` schemas into self-contained JSON Schema. |
| `validation.go` | Validates tool-call arguments against the tool's input schema. |
| `origin.go` | `Origin`-header check for browser DNS-rebinding safety. |
| `result.go` | Normalizes API responses into object-shaped `structuredContent` and builds error results. |

## Adding a tool

1. Add the OpenAPI `operationId` to `curatedOperationIDs` in `catalog.go`.
2. If the operation is **not** in the generated OpenAPI, add or adjust the
   proto/API surface first, then regenerate:

   ```bash
   cd proto && buf generate
   ```

3. Extend the tests in `catalog_test.go` / `service_test.go` to cover the new
   tool.

Never hand-edit `proto/gen/openapi.yaml` or other generated output — change the
proto definitions and regenerate.

## Testing

```bash
go test ./server/mcp/...
```

- `openapi_test.go` — spec parsing, registry building, `$ref` resolution.
- `catalog_test.go` — tool selection, naming, schema and annotation building.
- `adapter_test.go` — request construction and in-process execution (`adapter.go`), including the client-address peer, plus result normalization and error shaping (`result.go`).
- `validation_test.go` — argument validation against input schemas.
- `service_test.go` — the origin-header check, the stateless `2026-07-28`
  flow (`server/discover`, `tools/list`, `tools/call` with MCP headers), the
  request body limit, client-address forwarding through the in-process hop,
  plus the legacy end-to-end MCP protocol (`initialize`, `tools/list`,
  `tools/call`) confirming object-shaped `structuredContent`.

## Design notes

- **Two-layer input validation.** `validateToolArguments` runs a hand-rolled
  structural check (`validateSchemaValue`) and then the `google/jsonschema-go`
  validator. The first yields friendly messages; the second is the
  spec-complete backstop.
- **Embedded vs. file load.** Production reads the spec from
  `proto.OpenAPIYAML()` (`loadMCPServiceOpenAPISpec`). The path-based
  `loadOpenAPISpec` in `openapi.go` exists for tests.
- **Tools only.** The server advertises no prompts or resources in this version.
