# core

Business rules that do not care about HTTP or SQL. A package lands here when it
needs several store calls, or a decision, that more than one transport must
apply the same way.

| Package | Owns |
| --- | --- |
| `access/` | author-only access to private notes and comments |
| `mcpconnection/` | permissions, status and credentials for MCP connections |
| `messagecapture/` | validating and encrypting Telegram, WeCom and Telegram proxy settings |
| `memopayload/` | rebuilding a memo's derived payload from its markdown |
| `memoexport/` | the Memos Export Format container and records used for export and import |
| `instancesetting/` | the rules each instance setting group must meet before it is stored, and API↔store conversion |
| `user/` | the rules a username and password must meet before they are stored |
| `configuration/` | declarative configuration: reading files (substitution, strict decoding, schema checks, merging), planning and applying them against the instance, and the API ownership guard |

`instancesetting` converts between API and store messages for declarative
configuration. API responses redact secrets in `server/api`.
A write path's own conveniences, such as the API treating an empty secret as
"keep the stored one", run before validation, never inside it.

Layering: `core` may import `store`, `markdown`, `filter`,
`proto/gen`, and `internal`. It must not import `server` or `cmd`. New
resource-specific rules extracted from `server/api` belong here, one package
per resource.
