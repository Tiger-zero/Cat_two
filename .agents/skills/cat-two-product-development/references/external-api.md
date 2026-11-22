# Cat_two 外部接口

AI 客户端用 MCP 查找、整理和写回笔记。脚本、附件传输及回收站操作用 REST。下列路径以实例基址为起点，REST 前缀为 `/api`。部署版本可先查 `GET /api/instance/profile` 返回的 `version` 和 `commit`。旧镜像的工具以实际发现结果为准。 [REST 接口定义](../../../../proto/gen/openapi.yaml) · [API 变更记录](../../../../proto/api/CHANGELOG.md)

## 凭据和可见性

- **登录防护** 同一 IP 连续 6 次密码验证失败会被禁止发起新的登录。第 6 次及之后的登录请求返回 `403` 和 `LOGIN_IP_BLOCKED`。登录和续期用标准 `Set-Cookie` 和 `Cookie` 处理刷新令牌。每次登录有独立会话 ID，退出或撤销该会话后，后续请求会拒绝其访问和刷新凭据。黑名单由管理员会话在网页中维护。 [解封说明](../../../../README.md#登录防护和-ip-解封)
- **MCP** 连接到 `/mcp`，使用 Streamable HTTP，在每次请求发送 `Authorization: Bearer <MCP 连接令牌>`。在「设置 → MCP 和 API」创建，支持 7 天（默认）、14 天、1 个日历年或永久。创建时可选 `read_only` 或 `read_write`，默认和旧连接为读写。到期或删除后下一次请求会被拒绝。连接管理的 `/api/mcpConnections` 要求网页登录会话。 [连接说明](../../../../server/mcp/README.md#connecting-a-client) · [管理权限](../../../../server/api/mcp_connection_handler.go)
- **REST** 在同一设置页创建个人访问令牌（PAT），发送 `Authorization: Bearer <PAT>`，先用 `GET /api/auth/me` 核对归属。PAT 的权限按持有账户及各接口的权限检查决定，目前无逐令牌的只读、按笔记或按标签授权范围。可为各集成单独发放和撤销令牌。密码修改需要交互会话。MCP 连接令牌仅用于 `/mcp`。 [PAT 字段](../../../../proto/api/user_service.proto) · [认证](../../../../server/auth/authenticator.go) · [密码修改约束](../../../../server/api/user_service.go)
- **数据** 仅活跃的笔记作者可读写其笔记。REST 和 MCP 返回的笔记可见性为 `PRIVATE`，旧链接无法分享笔记。笔记链接 `/memos/<uid>` 仍要求登录并核对作者。匿名接口以 ACL 为准，例如实例概况可匿名读取。上游定义里仍保留部分已停用的空间、身份提供方和 Webhook 路由，个人模式调用会返回 `UNIMPLEMENTED`。 [笔记权限](../../../../core/access/memo.go) · [匿名路由](../../../../server/api/acl_config.go) · [停用路由](../../../../server/api/retired_modules.go)

网页使用 Connect `AuthService`。对应的 REST 路径为 `POST /api/auth/signin`、`POST /api/auth/refresh`、`GET /api/auth/me` 和 `POST /api/auth/signout`。Connect 与 REST 调用同一会话实现，共用密码校验、刷新 Cookie 和会话撤销。 [认证协议](../../../../proto/api/auth_service.proto) · [Connect 入口](../../../../server/api/connect_services.go) · [会话实现](../../../../server/api/auth_service_session.go)

## MCP 查找与共同创作

用 `tools/list` 发现实际工具及 JSON Schema，以 `auth_get_current_user` 核对账号。服务器仅提供工具能力。工具返回 `isError` 时按失败处理，成功时读取 `structuredContent`。 [服务实现](../../../../server/mcp/service.go) · [工具说明](../../../../server/mcp/README.md#tool-surface)

| 工具 | 参数和范围 |
| --- | --- |
| `auth_get_current_user` | 空对象 `{}`。 |
| `memo_search` | 搜索未归档的主笔记。`query` 是忽略大小写的字面子串，`tag` 含下级标签。`hasTags` 按有无标签筛选。可同时传 `startDate`、`endDate`（上海时区创建日期，`YYYY-MM-DD`，含当天）、`hasIncompleteTasks`、`hasAttachments`、`pinned`（布尔值）及 `creationChannel`（`web`、`telegram`、`browser_extension`、`wechat`、`unknown`）。`orderBy` 支持 `create_time` 或 `update_time` 加 `asc` 或 `desc`，默认 `create_time desc`。每页 1 至 50 条，默认 20。 |
| `memo_list_memos` | 按 REST 列表的 `filter`、`state`、`orderBy`、`pageSize`、`pageToken` 查询。可列归档笔记，兼容缺少 `memo_search` 的镜像。 |
| `memo_batch_get` | `names` 接受 1 至 50 个 `memos/<uid>` 字符串。返回 `memos`（含修订号）和 `unavailable`，支持已归档正文。每批正文合计最多 4 MiB，超出时拆分请求。找不到的、其他人所有的和回收站里的名称都会进入 `unavailable`。 |
| `memo_get_memo` | 传 `{"memo":"memos/<uid>"}` 读取正文和附件元数据。也可用于旧镜像逐条读取。 |
| `tag_tree` / `tag_search` | 读取未归档主笔记里的标签路径与含下级路径的笔记数。用整数 `offset` 和 `pageSize` 翻页，默认 0 和 50，每页最多 200。从 `nextOffset` 继续。`tag_search` 另需非空字符串 `query`。 |
| `memo_create_memo` | 传 `{"body":{"content":"# 标题\n\n- [ ] 待办\n\n#项目/Cat_two"}}`。可用 `body.createTime` 指定 RFC 3339 创建时间。设置顶层 `memoId` 和 `idempotent: true` 后，可重试相同请求。 |
| `memo_update_memo` | 传 `{"memo":"memos/<uid>","updateMask":"content","expectedRevision":"读取时的修订号","body":{"content":"完整正文"}}`。只更新掩码选定的字段。旧修订号会返回工具冲突错误。 |

`memo_search` 和 `memo_list_memos` 查下一页时，把返回的 `nextPageToken` 作为 `pageToken` 并沿用查询条件与顺序。写回前展示完整 Markdown，取得用户确认。更新原笔记时使用读取结果中的 `revision`，发生冲突后重新读回并核对预览。创建请求响应中断时可用原 `memoId`、`idempotent` 和原始字段重试，匹配时返回已保存的记录。MCP 使用顶层参数，REST 使用查询参数。 [分页及写入工作流](../../../../plugins/cat-two-notes/skills/organize-notes/SKILL.md) · [新建参数](../../../../proto/api/memo_service.proto)

## REST 笔记、附件和回收站

用 JSON 的 camelCase 字段。正文是完整 Markdown 字符串 `content`，在正文里写 `#标签/子标签`、任务和链接。读取列表及详情时，用 `content` 作原文，`snippet` 是摘要。列表默认仅返回 `NORMAL` 主笔记，传 `state=ARCHIVED` 查看已归档的主笔记。 `pageSize` 默认 50、上限 1000，复用 `nextPageToken` 翻页。CEL 表达式放在 `filter`，例如 `has_incomplete_tasks && "项目/Cat_two" in tags`。 [笔记协议](../../../../proto/api/memo_service.proto) · [CEL 语法](../../../../filter/README.md) · [正文格式](../SKILL.md#支持的内容格式)

保存笔记引用时使用稳定的 `memos/<uid>`，浏览器详情路径为 `/memos/<uid>`。服务端可自动生成 `uid`。客户端也可在创建时指定 `memoId`，用作资源名末段和幂等创建标识。同一请求重试保留这个 ID 和原请求字段。 [资源标识和重试](../SKILL.md#笔记标识与写入)

| 操作 | HTTP 调用 |
| --- | --- |
| 查找或读取笔记 | `GET /api/memos` 或 `GET /api/memos/<uid>`。资源名是 `memos/<uid>`。 |
| 新建或更新笔记 | `POST /api/memos` 直接提交 `{"content":"Markdown 正文"}`。可传 `memoId` 和 `idempotent=true` 进行幂等创建。`PATCH /api/memos/<uid>?updateMask=content&expectedRevision=<revision>` 提交完整新正文。修订号冲突返回 `409`。 |
| 归档或取消归档 | `PATCH /api/memos/<uid>?updateMask=state`，正文 `{"state":"ARCHIVED"}` 或 `{"state":"NORMAL"}`。也可用 MCP 的更新工具传同一掩码和 `body.state`。 |
| 移入或移出回收站 | `DELETE /api/memos/<uid>` 把笔记和回复移入回收站，保留 30 天。`GET /api/trash` 分页返回 `memos: [{memo, deletedTime}]`。`POST /api/trash/<uid>/restore` 恢复删除前的正常或归档状态。`DELETE /api/trash/<uid>` 永久删除。 |
| 读写附件 | `GET /api/memos/<uid>/attachments` 或 `GET /api/attachments/<uid>` 查元数据。`GET /file/attachments/<uid>/<filename>` 下载文件。用 `POST /api/attachments` 传 `filename`、MIME `type` 和 Base64 `content`，也可在 `POST /api/attachments:upload` 分块上传。保存笔记时在 `attachments` 中引用返回的 `attachments/<uid>` 名称。 |
| 修改附件集合 | `PATCH /api/memos/<uid>/attachments` 传 `{"attachments":[{"name":"attachments/<uid>"}]}`。提交的是完整集合，增添附件时也要带上原有引用。笔记正文中的托管附件链接必须与绑定一致。 |
| ZIP 导出与导入 | `GET /file/users/<username>/export` 导出自己的笔记。可用多个 `memo=<uid>` 选择最多 100 条。`POST /api/users/<username>:importMemos` 分块上传 ZIP，结束时用 `validateOnly: true` 预演。 |

独立附件由上传者访问，绑定后的附件随笔记权限。个人 MCP 提供附件元数据。上传、下载文件、删除笔记及回收站操作使用上表的 HTTP 路径。 [附件协议](../../../../proto/api/attachment_service.proto) · [附件绑定](../../../../server/api/memo_attachment_service.go) · [文件路由](../../../../server/fileserver/fileserver.go) · [回收站实现](../../../../server/api/memo_trash_handler.go) · [ZIP 导出](../../../../server/fileserver/memo_export.go) · [导入协议](../../../../proto/api/user_service.proto)

## 统计和标签

- 日历与侧栏使用 Connect `UserService.ListUserStats` 取得授权的时间戳和标签统计，在网页按 `Asia/Shanghai` 聚合。REST 对应 `GET /api/users/<username>/stats`，也可用 `users/-` 作为父资源，仍按调用者可见性检查。`GetUserStats` 对应 `GET /api/users/<username>:getStats`。 [统计协议](../../../../proto/api/user_service.proto) · [权限与查询](../../../../server/api/user_service_stats.go) · [日期聚合](../../../../web/src/hooks/useFilteredMemoStats.ts)
- `POST /api/tags/rename` 接收 `oldName` 和 `newName`，按完整路径移动分支。`POST /api/tags/delete` 接收 `tagName` 及 `mode`。`remove` 从正文移除该分支的标签，`trash` 将匹配的笔记移入回收站。`POST /api/tags/normalize` 整理斜杠两侧的空格。操作限定当前账户，成功时返回 `updatedCount`。 [标签写入](../../../../server/api/memo_tag_routes.go)
- Web、REST 和 MCP 的分支统计对每篇笔记只计一次，并兼容缺少父路径的旧索引。查找无标签笔记使用 REST 的 `size(tags) == 0`，或给 MCP `memo_search` 传 `hasTags: false`。 [分支计数](../../../../core/memopayload/tags.go) · [MCP 标签工具](#mcp-查找与共同创作)

## 变更通知和收件

- 需要即时刷新时，使用 Bearer 认证连接 `GET /api/sse`。收到 `{"type":"memo.changed"}` 后通过列表或详情重新读取。通知只包含事件类型。SSE 只发送连接期间的事件，断线重连后重新查询。个人模式停用了通用出站 Webhook 管理和发送。 [SSE 路由](../../../../server/api/sse_handler.go) · [事件定义](../../../../server/api/sse_hub.go) · [Webhook 发送限制](../../../../server/api/memo_service_webhooks.go)
- Telegram Bot 私聊使用长轮询，企业微信自建应用使用 `/integrations/wecom` 接收签名加密回调。启用前按部署说明配置发件人。接入只保存纯文本，由唯一活跃管理员持有，加入渠道标签，按消息 ID 去重，成功后回复 `Yes!`。日常 AI 或浏览器集成可直接用 MCP 或 REST 写入。 [收件配置](../../../../README.md#telegram-和企业微信收件) · [写入与去重](../../../../server/api/memo_capture.go)
- 「通知」和「网络」使用 `/api/notificationSettings` 和 `/api/networkSettings` 的 `GET` 与 `PATCH`，要求活跃的管理员网页登录会话。读取返回启用状态、来源、渠道标识及凭据是否已设置。代理 URL 和渠道密钥为只写字段，更新时留空保留旧值。 [设置接口](../../../../server/api/capture_settings.go)
- 企业微信配置中的 `wecom.callbackUrl` 是可读的手动公网地址。`PATCH` 省略该字段时保留原值，传空字符串时清除。`wecomDefaultCallbackUrl` 来自符合格式的实例公网域名。两者都支持 HTTP 或 HTTPS 和显式端口，填写裸域名时默认使用 HTTPS。保存只校验格式，公网连通性以企业微信后台的 GET 验证为准。 [地址格式](../../../../core/messagecapture/wecom_callback.go) · [接收消息与事件](https://developer.work.weixin.qq.com/document/path/90238)
- 通知更新提交一条渠道，`telegram` 用 `enabled`、`userId` 和 `botToken`，`wecom` 用 `enabled`、`corpId`、`agentId`、`userId`、`callbackToken` 和 `encodingAesKey`。首次保存前各渠道读取容器环境，手动启停按保存值生效。企业微信回调路径由通知设置返回。 [通知配置](../../../../core/messagecapture/configuration.go)
- 网络更新提交 `enabled` 和可选的 `proxyUrl`，接受 HTTP、HTTPS、SOCKS5 和 SOCKS5H。代理用于 Telegram 轮询和回复。未保存时遵循容器的环境代理与 `NO_PROXY`，保存为停用时直连。 [代理规则](../../../../core/messagecapture/network.go) · [部署配置](../../../../README.md#telegram-网络代理)

## 共创能力演进

批量整理结果使用可追溯的来源链接和精简的变更记录。日常应用内刷新走 SSE，需要可靠的跨服务通知时再单独定义事件内容、签名、重试和断线补偿。
