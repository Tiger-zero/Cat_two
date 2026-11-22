# Cat_two 外部接口

AI 客户端用 MCP 查找、整理和写回笔记。脚本、附件传输及回收站操作用 REST。下列路径以实例基址为起点，REST 前缀为 `/api`。部署版本可先查 `GET /api/instance/profile` 返回的 `version` 和 `commit`。旧镜像的工具以实际发现结果为准。 [REST 接口定义](../../../../proto/gen/openapi.yaml) · [API 变更记录](../../../../proto/api/CHANGELOG.md)

## 凭据和可见性

- **MCP** 连接到 `/mcp`，使用 Streamable HTTP，在每次请求发送 `Authorization: Bearer <MCP 连接令牌>`。在「设置 → MCP 和 API」创建，支持 7 天（默认）、14 天、1 个日历年或永久。到期或删除后下一次请求会被拒绝。连接管理的 `/api/mcpConnections` 要求网页登录会话。 [连接说明](../../../../server/mcp/README.md#connecting-a-client) · [管理权限](../../../../server/api/mcp_connection_handler.go)
- **REST** 在同一设置页创建个人访问令牌（PAT），发送 `Authorization: Bearer <PAT>`，先用 `GET /api/auth/me` 核对归属。PAT 的权限按持有账户及各接口的权限检查决定，目前无逐令牌的只读、按笔记或按标签授权范围。可为各集成单独发放和撤销令牌。密码修改需要交互会话。MCP 连接令牌仅用于 `/mcp`。 [PAT 字段](../../../../proto/api/user_service.proto) · [认证](../../../../server/auth/authenticator.go) · [密码修改约束](../../../../server/api/user_service.go)
- **数据** 仅活跃的笔记作者可读写其笔记。REST 和 MCP 返回的笔记可见性为 `PRIVATE`，旧链接无法分享笔记。笔记链接 `/memos/<uid>` 仍要求登录并核对作者。匿名接口以 ACL 为准，例如实例概况可匿名读取。上游定义里仍保留部分已停用的空间、身份提供方和 Webhook 路由，个人模式调用会返回 `UNIMPLEMENTED`。 [笔记权限](../../../../core/access/memo.go) · [匿名路由](../../../../server/api/acl_config.go) · [停用路由](../../../../server/api/retired_modules.go)

## MCP 查找与共同创作

用 `tools/list` 发现实际工具及 JSON Schema，以 `auth_get_current_user` 核对账号。服务器仅提供工具能力。工具返回 `isError` 时按失败处理，成功时读取 `structuredContent`。 [服务实现](../../../../server/mcp/service.go) · [工具说明](../../../../server/mcp/README.md#tool-surface)

| 工具 | 参数和范围 |
| --- | --- |
| `auth_get_current_user` | 空对象 `{}`。 |
| `memo_search` | 搜索未归档的主笔记。`query` 是忽略大小写的字面子串，`tag` 含下级标签。可同时传 `startDate`、`endDate`（上海时区创建日期，`YYYY-MM-DD`，含当天）、`hasIncompleteTasks`、`hasAttachments`、`pinned`（布尔值）及 `creationChannel`（`web`、`telegram`、`browser_extension`、`wechat`、`unknown`）。`orderBy` 支持 `create_time` 或 `update_time` 加 `asc` 或 `desc`，默认 `create_time desc`。每页 1 至 50 条，默认 20。 |
| `memo_list_memos` | 按 REST 列表的 `filter`、`state`、`orderBy`、`pageSize`、`pageToken` 查询。可列归档笔记，兼容缺少 `memo_search` 的镜像。 |
| `memo_batch_get` | `names` 接受 1 至 50 个 `memos/<uid>` 字符串。返回 `memos` 和 `unavailable`，支持已归档正文。找不到的、其他人所有的和回收站里的名称都会进入 `unavailable`。 |
| `memo_get_memo` | 传 `{"memo":"memos/<uid>"}` 读取正文和附件元数据。也可用于旧镜像逐条读取。 |
| `tag_tree` / `tag_search` | 读取未归档主笔记里的标签路径与含下级路径的笔记数。用整数 `offset` 和 `pageSize` 翻页，默认 0 和 50，每页最多 200。从 `nextOffset` 继续。`tag_search` 另需非空字符串 `query`。 |
| `memo_create_memo` | 传 `{"body":{"content":"# 标题\n\n- [ ] 待办\n\n#项目/Cat_two"}}`。可用 `body.createTime` 指定 RFC 3339 创建时间。 |
| `memo_update_memo` | 传 `{"memo":"memos/<uid>","updateMask":"content","body":{"content":"完整正文"}}`。只更新掩码选定的字段。 |

`memo_search` 和 `memo_list_memos` 查下一页时，把返回的 `nextPageToken` 作为 `pageToken` 并沿用查询条件与顺序。写回前展示完整 Markdown，取得用户确认。更新原笔记时重读目标，若内容已变更则重新核对预览。新建时可为同一次操作固定一个 `memoId`，MCP 使用顶层参数，REST 使用查询参数。响应中断或重试返回已存在时，先按该 ID 读回确认。 [分页及写入工作流](../../../../plugins/cat-two-notes/skills/organize-notes/SKILL.md) · [新建参数](../../../../proto/api/memo_service.proto)

## REST 笔记、附件和回收站

用 JSON 的 camelCase 字段。正文是完整 Markdown 字符串 `content`，在正文里写 `#标签/子标签`、任务和链接。读取列表及详情时，用 `content` 作原文，`snippet` 是摘要。列表默认仅返回 `NORMAL` 主笔记，传 `state=ARCHIVED` 查看已归档的主笔记。 `pageSize` 默认 50、上限 1000，复用 `nextPageToken` 翻页。CEL 表达式放在 `filter`，例如 `has_incomplete_tasks && "项目/Cat_two" in tags`。 [笔记协议](../../../../proto/api/memo_service.proto) · [CEL 语法](../../../../filter/README.md) · [正文格式](../SKILL.md#支持的内容格式)

| 操作 | HTTP 调用 |
| --- | --- |
| 查找或读取笔记 | `GET /api/memos` 或 `GET /api/memos/<uid>`。资源名是 `memos/<uid>`。 |
| 新建或更新笔记 | `POST /api/memos` 直接提交 `{"content":"Markdown 正文"}`。`PATCH /api/memos/<uid>?updateMask=content` 按相同 JSON 格式提交完整新正文。 |
| 归档或取消归档 | `PATCH /api/memos/<uid>?updateMask=state`，正文 `{"state":"ARCHIVED"}` 或 `{"state":"NORMAL"}`。也可用 MCP 的更新工具传同一掩码和 `body.state`。 |
| 移入或移出回收站 | `DELETE /api/memos/<uid>` 把笔记和回复移入回收站，保留 30 天。`GET /api/trash` 分页返回 `memos: [{memo, deletedTime}]`。`POST /api/trash/<uid>/restore` 恢复删除前的正常或归档状态。`DELETE /api/trash/<uid>` 永久删除。 |
| 读写附件 | `GET /api/memos/<uid>/attachments` 或 `GET /api/attachments/<uid>` 查元数据。`GET /file/attachments/<uid>/<filename>` 下载文件。用 `POST /api/attachments` 传 `filename`、MIME `type` 和 Base64 `content`，也可在 `POST /api/attachments:upload` 分块上传。保存笔记时在 `attachments` 中引用返回的 `attachments/<uid>` 名称。 |
| 修改附件集合 | `PATCH /api/memos/<uid>/attachments` 传 `{"attachments":[{"name":"attachments/<uid>"}]}`。提交的是完整集合，增添附件时也要带上原有引用。笔记正文中的托管附件链接必须与绑定一致。 |
| ZIP 导出与导入 | `GET /file/users/<username>/export` 导出自己的笔记。可用多个 `memo=<uid>` 选择最多 100 条。`POST /api/users/<username>:importMemos` 分块上传 ZIP，结束时用 `validateOnly: true` 预演。 |

独立附件由上传者访问，绑定后的附件随笔记权限。个人 MCP 提供附件元数据。上传、下载文件、删除笔记及回收站操作使用上表的 HTTP 路径。 [附件协议](../../../../proto/api/attachment_service.proto) · [附件绑定](../../../../server/api/memo_attachment_service.go) · [文件路由](../../../../server/fileserver/fileserver.go) · [回收站实现](../../../../server/api/memo_trash_handler.go) · [ZIP 导出](../../../../server/fileserver/memo_export.go) · [导入协议](../../../../proto/api/user_service.proto)

## 变更通知和收件

- 需要即时刷新时，使用 Bearer 认证连接 `GET /api/sse`。收到 `{"type":"memo.changed"}` 后通过列表或详情重新读取。通知只包含事件类型。SSE 只发送连接期间的事件，断线重连后重新查询。个人模式停用了通用出站 Webhook 管理和发送。 [SSE 路由](../../../../server/api/sse_handler.go) · [事件定义](../../../../server/api/sse_hub.go) · [Webhook 发送限制](../../../../server/api/memo_service_webhooks.go)
- Telegram Bot 私聊使用长轮询，企业微信自建应用使用 `/integrations/wecom` 接收签名加密回调。启用前按部署说明配置发件人。接入只保存纯文本，由唯一活跃管理员持有，加入渠道标签，按消息 ID 去重，成功后回复 `Yes!`。日常 AI 或浏览器集成可直接用 MCP 或 REST 写入。 [收件配置](../../../../README.md#telegram-和企业微信收件) · [写入与去重](../../../../server/api/memo_capture.go)

## 共创能力演进

1. 为 MCP 连接增加只读、可新建和可更新等权限选项，按连接展示权限、到期和最近使用时间。先确定已有连接的兼容策略，再在工具发现和执行两处验收。
2. 为写回增加版本条件和可查询的写入结果，覆盖不同工具共同编辑时的冲突、重复投递和断线重试。继续复用现有 Markdown、标签和附件绑定规则。
3. 批量整理结果使用可追溯的来源链接和精简的变更记录。日常应用内刷新走 SSE，需要可靠的跨服务通知时再单独定义事件内容、签名、重试和断线补偿。
