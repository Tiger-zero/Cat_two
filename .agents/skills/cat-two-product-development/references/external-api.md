# Cat_two 外部接口

AI 客户端和自动化通过 `/mcp` 查找、整理和写回笔记。Telegram 和企业微信用于收取消息。附件传输、回收站、ZIP 导入和导出由网页登录会话完成。升级后旧通用 REST 路径 `/api` 和 `/api/*` 返回 `404`，旧个人访问令牌（PAT）失效。旧镜像仍以其版本及发现结果为准。 [API 变更记录](../../../../proto/api/CHANGELOG.md) · [版本与部署](deployment.md#备份和升级)

## 凭据和可见性

- **登录防护** 同一 IP 连续 6 次密码验证失败会被禁止发起新的登录。第 6 次及之后的登录请求返回 `403` 和 `LOGIN_IP_BLOCKED`。登录和续期用标准 `Set-Cookie` 和 `Cookie` 处理刷新令牌。每次登录有独立会话 ID，退出或撤销该会话后，后续请求会拒绝其访问和刷新凭据。黑名单由管理员会话在网页中维护。 [解封说明](deployment.md#登录防护和-ip-解封)
- **MCP** 连接到 `/mcp`，使用 Streamable HTTP，在每次请求发送 `Authorization: Bearer <MCP 连接令牌>`。在「设置 → MCP」创建，支持 7 天（默认）、14 天、1 个日历年或永久。创建时可选 `read_only` 或 `read_write`，默认和旧连接为读写。到期或删除后下一次请求会被拒绝。给各个客户端分别创建连接，可单独删除。 [连接说明](../../../../server/mcp/README.md#connecting-a-client) · [管理权限](../../../../server/api/mcp_connection_handler.go)
- **数据** 仅活跃的笔记作者可读写其笔记。网页和 MCP 提供当前账户的私有笔记。详情链接也需要登录和作者权限。网页登录前使用 Connect 实例概况等白名单方法。 [笔记权限](../../../../core/access/memo.go) · [匿名路由](../../../../server/api/acl_config.go)

网页通过 Connect `AuthService` 登录、刷新及退出。查看部署的 `version` 和 `commit` 时，在浏览器开发者工具的网络请求中读取 `InstanceService.GetInstanceProfile` 的响应。 [认证协议](../../../../proto/api/auth_service.proto) · [Connect 入口](../../../../server/api/connect_services.go) · [会话实现](../../../../server/api/auth_service_session.go)

网页在编辑时向 `POST /internal/session/activity` 上报活动。请求使用当前站点的浏览器 `Origin` 和网页访问令牌的 Bearer 请求头，无需请求体。成功时返回 `idleExpiresAt`。服务端以最近一次登录或编辑活动后的 45 分钟作为空闲期限，刷新 Cookie 或访问令牌不会顺延此期限。过期后原会话无法再次上报活动。 [接口检查](../../../../server/api/auth_session_activity.go) · [会话持久化](../../../../store/user_session.go)

## MCP 查找与共同创作

用 `tools/list` 发现实际工具及 JSON Schema，以 `auth_get_current_user` 核对账号。服务器仅提供工具能力。工具返回 `isError` 时按失败处理，成功时读取 `structuredContent`。 [服务实现](../../../../server/mcp/service.go) · [工具说明](../../../../server/mcp/README.md#tool-surface)

| 工具 | 参数和范围 |
| --- | --- |
| `auth_get_current_user` | 空对象 `{}`。 |
| `memo_search` | 搜索回收站以外的主笔记。`query` 是忽略大小写的字面子串，`tag` 含下级标签。`hasTags` 按有无标签筛选。可同时传 `startDate`、`endDate`（上海时区创建日期，`YYYY-MM-DD`，含当天）、`hasIncompleteTasks`、`hasAttachments`、`pinned`（布尔值）及 `creationChannel`（`web`、`telegram`、`browser_extension`、`wechat`、`unknown`）。`orderBy` 支持 `create_time` 或 `update_time` 加 `asc` 或 `desc`，默认 `create_time desc`。每页 1 至 50 条，默认 20。 |
| `memo_list_memos` | 按 `filter`、`orderBy`、`pageSize`、`pageToken` 查询。`filter` 接受 CEL。返回当前账户的笔记。 |
| `memo_batch_get` | `names` 接受 1 至 50 个 `memos/<uid>` 字符串。返回 `memos`（含修订号）和 `unavailable`。每批正文合计最多 4 MiB，超出时拆分请求。找不到的、其他人所有的和回收站里的名称都会进入 `unavailable`。 |
| `memo_get_memo` | 传 `{"memo":"memos/<uid>"}` 读取正文和附件元数据。也可用于旧镜像逐条读取。 |
| `tag_tree` / `tag_search` | 读取回收站以外主笔记的标签路径与含下级路径的笔记数。用整数 `offset` 和 `pageSize` 翻页，默认 0 和 50，每页最多 200。从 `nextOffset` 继续。`tag_search` 另需非空字符串 `query`。 |
| `memo_create_memo` | 传 `{"body":{"content":"# 标题\n\n- [ ] 待办\n\n#项目/Cat_two"}}`。省略 `body.createTime` 时由服务端在首次成功保存时记录创建时间。导入历史笔记的客户端也可在创建请求中显式传入 RFC 3339 时间。设置顶层 `memoId` 和 `idempotent: true` 后，可重试相同请求。 |
| `memo_update_memo` | 传 `{"memo":"memos/<uid>","updateMask":"content","expectedRevision":"读取时的修订号","body":{"content":"完整正文"}}`。只更新掩码选定的字段。旧修订号会返回工具冲突错误。 |

`memo_search` 和 `memo_list_memos` 每页返回最多 4 MiB 的 Markdown。达到上限时，即使本页条数少于 `pageSize`，也请用 `nextPageToken` 继续，并沿用查询条件与顺序。若单条超过上限，按错误提示用 `memo_get_memo` 读取该笔记，再用错误中提供的 `pageToken` 继续。分页请求遇到并发修改时可用相同游标重试。

写回前展示完整 Markdown，取得用户确认。更新原笔记时使用读取结果中的 `revision`，发生冲突后重新读回并核对预览。创建请求响应中断时可用原 `memoId`、`idempotent` 和原始字段重试，匹配时返回已保存的记录。 [分页及写入工作流](../../../../plugins/cat-two-notes/skills/organize-notes/SKILL.md) · [新建参数](../../../../proto/api/memo_service.proto)

## 令牌创建笔记

通过 `memo_create_memo` 新建笔记时，服务端将 `#MCP连接` 写入保存的 Markdown 正文。正文已经解析出路径恰为 `MCP连接` 的标签时沿用原文。幂等重试继续提交原请求字段及相同的 `memoId`，匹配时返回已保存的笔记。 [笔记新建入口](../../../../server/api/memo_service.go) · [创建重试](../../../../server/api/memo_create_helpers.go)

该规则仅用于上述新建请求。网页登录创建沿用提交的正文，Telegram 和企业微信收件各自添加渠道标签。后续编辑可按普通标签修改或移除 `#MCP连接`，旧笔记正文按原样保留。可用侧栏标签筛选，或通过 MCP `memo_search` 的 `tag: "MCP连接"` 找到这类笔记。 [标签筛选](#统计和标签) · [收件渠道](#变更通知和收件)

## 网页接口

网页在 `/memos.api.*` 使用 Connect，并通过 `/internal/` 处理下列原生 HTTP 请求。这些路由是网页实现的一部分。需要账户数据时，使用同源网页的有效会话。`/file/` 提供附件、头像和 ZIP 下载，并核对相应的会话及作者权限。 [网页传输](../../../../web/src/connect.ts) · [路由](../../../../server/api/api.go) · [文件服务](../../../../server/fileserver/README.md)

| 网页操作 | 请求 |
| --- | --- |
| 笔记录入、修改和移入回收站 | Connect `MemoService`。正文为 Markdown，更新提交字段掩码和读到的修订号。 |
| 查看和管理回收站 | `GET /internal/trash`、`POST /internal/trash/{uid}/restore`、`DELETE /internal/trash/{uid}`。 |
| 附件上传与绑定 | Connect `AttachmentService` 和 `MemoService`。分块上传用 `UploadAttachment`，绑定提交完整附件集合。 |
| ZIP 导出与导入 | `GET` 或 `HEAD /file/users/{username}/export`，选导时用重复的 `memo` 参数，最多 100 条。ZIP 分块导入用 Connect `UserService.ImportMemos`，可先预演。 |
| 两步验证与登录封禁 | `GET /internal/auth/totp`，`POST /internal/auth/totp/{setup,enable,disable}`。`GET /internal/auth/loginBlocklist`，`DELETE /internal/auth/loginBlocklist/{ip}`。 |
| MCP 连接设置 | `GET`、`POST /internal/mcpConnections`，`DELETE /internal/mcpConnections/{id}`。 |

个人 MCP 可以返回附件元数据。上传、下载附件正文、删除笔记、回收站和 ZIP 操作在网页登录后完成。[附件协议](../../../../proto/api/attachment_service.proto) · [回收站实现](../../../../server/api/memo_trash_handler.go) · [ZIP 导出](../../../../server/fileserver/memo_export.go) · [导入协议](../../../../proto/api/user_service.proto)

## 统计和标签

- 日历与侧栏使用 Connect `UserService.ListUserStats` 取得当前账号的日期与标签统计，在网页按 `Asia/Shanghai` 显示。 [统计协议](../../../../proto/api/user_service.proto) · [权限与查询](../../../../server/api/user_service_stats.go) · [日期聚合](../../../../web/src/hooks/useFilteredMemoStats.ts)
- 网页通过 `POST /internal/tags/{rename,delete,normalize}` 移动标签分支、从正文移除标签、将匹配笔记移入回收站或整理斜杠两侧的空格。操作限定当前账户。 [标签写入](../../../../server/api/memo_tag_routes.go)
- 网页和 MCP 的分支统计对每篇笔记只计一次，并兼容缺少父路径的旧索引。网页可用「无标签」筛选，MCP `memo_search` 可传 `hasTags: false`。 [分支计数](../../../../core/memopayload/tags.go) · [MCP 标签工具](#mcp-查找与共同创作)

## 变更通知和收件

- 网页连接 `GET /internal/sse` 接收变更通知。收到 `{"type":"memo.changed"}` 后重新读取列表或详情。通知只包含事件类型。SSE 只发送连接期间的事件，断线重连后重新查询。 [SSE 路由](../../../../server/api/sse_handler.go) · [事件定义](../../../../server/api/sse_hub.go)
- Telegram Bot 私聊通过长轮询主动接收消息。企业微信自建应用走 `/integrations/wecom` 的 `GET` 验证及 `POST` 回调，校验签名并解密消息。启用前按部署说明配置发件人。接入只保存纯文本，由唯一活跃管理员持有，加入渠道标签，按消息 ID 去重，成功后回复 `Yes!`。 [收件配置](deployment.md#telegram-和企业微信收件) · [写入与去重](../../../../server/api/memo_capture.go)
- 「通知」和「网络」使用 `/internal/notificationSettings` 和 `/internal/networkSettings` 的 `GET` 与 `PATCH`，要求活跃的管理员网页登录会话。读取返回启用状态、来源、渠道标识及凭据是否已设置。代理 URL 和渠道密钥为只写字段，更新时留空保留旧值。 [设置接口](../../../../server/api/capture_settings.go)
- 企业微信配置中的 `wecom.callbackUrl` 是可读的手动公网地址。`PATCH` 省略该字段时保留原值，传空字符串时清除。`wecomDefaultCallbackUrl` 来自符合格式的实例公网域名。两者都支持 HTTP 或 HTTPS 和显式端口，填写裸域名时默认使用 HTTPS。保存只校验格式，公网连通性以企业微信后台的 GET 验证为准。 [地址格式](../../../../core/messagecapture/wecom_callback.go) · [接收消息与事件](https://developer.work.weixin.qq.com/document/path/90238)
- 通知更新提交一条渠道，`telegram` 用 `enabled`、`userId` 和 `botToken`，`wecom` 用 `enabled`、`corpId`、`agentId`、`userId`、`callbackToken` 和 `encodingAesKey`。首次保存前各渠道读取容器环境，手动启停按保存值生效。企业微信回调路径由通知设置返回。 [通知配置](../../../../core/messagecapture/configuration.go)
- 网络更新提交 `enabled` 和可选的 `proxyUrl`，接受 HTTP、HTTPS、SOCKS5 和 SOCKS5H。代理用于 Telegram 轮询和回复。未保存时遵循容器的环境代理与 `NO_PROXY`，保存为停用时直连。 [代理规则](../../../../core/messagecapture/network.go) · [部署配置](deployment.md#telegram-网络代理)

## 共创能力演进

批量整理结果使用可追溯的来源链接和精简的变更记录。日常应用内刷新走 SSE，需要可靠的跨服务通知时再单独定义事件内容、签名、重试和断线补偿。
