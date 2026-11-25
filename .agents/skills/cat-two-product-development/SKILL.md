---
name: cat-two-product-development
description: 开发 Cat_two 单账户笔记的功能和业务逻辑。用于笔记、标签、附件、回收站、ZIP 导入导出、会话权限、SQLite、网页接口、MCP、Telegram 和企业微信集成。
---

# Cat_two 产品与业务开发

Cat_two 是单账户自托管笔记应用，用于记录、整理与同步笔记。正文使用 Markdown，笔记和附件按账户鉴权。核对部署实例时，从网页 Connect `InstanceService.GetInstanceProfile` 获取 `version` 和 `commit`。 [功能与实现](references/product-guide.md) · [外部接口](references/external-api.md)

Cat_two 的唯一数据库是 SQLite。仓库开发流程、验证和镜像交付使用总体开发 Skill。部署、升级和恢复见部署指南。桌面布局、颜色、字体、编辑器控件和交互视觉验收使用 UI 开发 Skill。操作部署中的笔记或配置 Codex 时使用 MCP Skill。 [总体开发](../cat-two-development/SKILL.md) · [部署与维护](references/deployment.md) · [桌面 UI 开发](../cat-two-ui-development/SKILL.md) · [MCP 使用](../cat-two-mcp/SKILL.md)

## 笔记标识与写入

- 已有笔记用稳定的 `memos/<uid>` 资源名引用，详情页为 `/memos/<uid>`。创建可以省略 `memoId` 由服务端生成，或传 1 至 36 位字母、数字及中间的连字符。数据库内部 ID 仅供存储层使用。编辑和从回收站恢复时沿用资源名。 [笔记协议](../../../proto/api/memo_service.proto) · [标识校验](../../../internal/identifier/resource_name.go)
- 网页 Connect 和个人 MCP 复用笔记服务的作者权限和写入校验。新建时一并保存正文、附件绑定及引用。创建重试沿用显式 `memoId`、`idempotent: true` 和原请求字段。同一账号以相同请求重试返回成功创建的笔记，已被使用但不匹配的 ID 返回冲突。网断后保留创建快照和已上传文件，避免重复创建或上传。 [新建](../../../server/api/memo_create_helpers.go) · [编辑器保存](../../../web/src/components/MemoEditor/edit/hooks/useMemoSave.ts)
- 通过个人 MCP 新建笔记时，服务端在 Markdown 正文中添加 `#MCP连接`。正文已包含解析后路径完全匹配的标签时沿用原文。自动添加只在笔记新建时执行。 [令牌创建笔记](references/external-api.md#令牌创建笔记)
- 更新使用读取时的 `revision` 作为 `expectedRevision`。`updateMask` 明确选中的字段，更新 `content` 时提交完整 Markdown。版本冲突时保留草稿和待上传附件，重新取回服务端内容后核对。局部修改继续尊重其他字段已有状态。 [更新协议](../../../proto/api/memo_service.proto) · [MCP 示例](references/external-api.md#mcp-查找与共同创作)
- 网页草稿首次成功保存时，由服务端记录笔记的 `createTime`。已有笔记在网页编辑器内的创建时间为只读，更新时间可调整。日期筛选、按日统计与 MCP 日期使用 `Asia/Shanghai`。侧栏日历仅筛选列表。标签路径用 `/` 分层，选父标签时纳入子路径。笔记列表支持关键词、CEL、来源筛选和排序，使用稳定分页参数。 [日期工具](../../../web/src/lib/calendar-utils.ts) · [统计和标签接口](references/external-api.md#统计和标签) · [CEL](../../../filter/README.md)

## 标签

- `#标签` 可以写在正文任意位置，多个标签用空格分开。斜杠分隔层级，`#读书/散文` 也会出现在「读书」分支。输入框的 `#` 按钮和键入 `#` 共用标签建议。 [多级标签参考](https://help.flomoapp.com/basic/tag.html)
- 侧栏树形和平铺都按名称排序，数字前缀可用于安排顺序。父标签筛选包含下级路径，统计对同一篇笔记只计一次。网页和 MCP 都可查找无标签笔记，正文的代码和链接内的 `#` 不构成标签。 [接口与计数](references/external-api.md#统计和标签)
- 标签菜单的重命名接受完整目标路径。删除分支时可移除标签并保留笔记，或将匹配笔记移入可恢复的回收站。操作包含下级标签。移动笔记保留原标签和关联内容。 [标签操作](../../../server/api/memo_tag_routes.go)

## 附件、回收站和 ZIP

- 分块上传或常规上传先生成 `attachments/<uid>` 资源名，再由笔记提交绑定。附件集合更新提交完整列表，正文里的托管附件链接必须与绑定一致。未绑定文件只对上传者开放，绑定后随笔记作者权限。纯附件笔记也可保存。 [上传协议](../../../proto/api/attachment_service.proto) · [绑定](../../../server/api/memo_attachment_service.go) · [文件服务](../../../server/fileserver/README.md)
- 笔记移入回收站后保留正文、附件及评论子树 30 天。恢复后重新出现在笔记列表。彻底删除以数据库提交为准，失败的文件清理由后台任务重试。 [回收站接口](../../../server/api/memo_trash_handler.go) · [存储](../../../store/memo_trash.go)
- `memos-export` 1.x ZIP 包含 `manifest.json`、每篇笔记的 Markdown 和 JSON 元数据及附件数据或外链。可导出整个账户或最多 100 条指定笔记。导入支持预演、保留现有、覆盖和创建副本，应结合逐条报告核对附件、引用、标签和评论层级。新建导入时一并保存正文、附件和置顶设置。旧版 ZIP 中标为归档的笔记作为普通笔记导入。 [归档格式](../../../core/memoexport/format.go) · [导入实现](../../../server/api/user_service_memo_import.go) · [恢复与升级](references/product-guide.md#数据备份和升级)

## 本地文件存储

- 个人模式 SQLite 管理账户、笔记元数据、标签、引用、正文内容哈希和搜索所需的正文副本。应用从数据目录下的 `notes/` 读取并校验 Markdown 文件，新附件写入 `assets/`。文件发布成功后提交 SQLite 中的引用。通过网页或 MCP 更新，保持文件、引用与搜索一致。 [正文文件](../../../store/memo_body.go) · [附件写入](../../../store/attachment_files.go)

## 账户与集成

- 首次设置创建唯一管理员，随后关闭网页注册。笔记和统计只对作者开放，旧资源按原所有者校验。网页登录、两步验证、Cookie 续期和退出以独立会话处理，同 IP 连续 6 次密码失败会持久封禁该地址。管理员可在「设置 → 安全」解封。 [登录与解封](references/deployment.md#登录防护和-ip-解封) · [账户与可见性](references/external-api.md#凭据和可见性)
- 网页登录会话以登录或最近一次笔记编辑为起点。满 45 分钟未编辑后，浏览器提示重新登录，服务端拒绝旧会话的访问和续期。浏览、搜索、滚动、同步和令牌轮换均不延长此期限。 [会话记录](../../../store/user_session.go) · [网页续期](references/external-api.md#凭据和可见性)
- 网页通过 Connect、`/internal/` 和 `/file/` 完成会话、笔记、设置和文件操作。`/mcp` 使用专用令牌，校验有效期、撤销及读写范围。设置和管理接口按网页会话校验对应权限。扩展 MCP 时核对已启用的工具、数据归属、分页、写入重试、限流和令牌撤销。网页实时更新用 SSE，断线后重新读取。 [外部接口](references/external-api.md) · [MCP 协议](../../../server/mcp/README.md) · [公网部署](references/deployment.md#在公网部署)
- Telegram 私聊和企业微信自建应用只接收授权发件人的文本，通过消息 ID 去重，保存成管理员的私有笔记并添加 `#Telegram` 或 `#微信`。保存成功后在原会话回复 `Yes!`。企业微信走签名加密的 `/integrations/wecom` 回调，Telegram 主动轮询。 [收件部署](references/deployment.md#telegram-和企业微信收件) · [写入和去重](../../../server/api/memo_capture.go)
- 企业微信公网回调可在「设置 → 通知」单独保存。域名可带端口，默认用 HTTPS 并补齐 `/integrations/wecom`。已明确填写的 HTTP 或 HTTPS URL 保留协议与端口，公网入口把请求转发给固定回调路由。企业微信保存配置时发出验签解密的 GET 请求，之后用 POST 推送消息。 [部署示例](references/deployment.md#企业微信自建应用) · [企业微信回调配置](https://developer.work.weixin.qq.com/document/path/90930)
- 「设置 → 通知」可分别启停两条收件渠道。首次保存前各自沿用容器配置，保存后按渠道即时生效。令牌和密钥留空保留原值。「设置 → 网络」配置 Telegram 轮询和回复的出站代理，首次保存前遵循 `HTTPS_PROXY` 和 `NO_PROXY`，手动关闭后直连。渠道密钥和代理 URL 加密存入实例数据库，查询返回状态、渠道标识和凭据是否已设置。 [设置接口](../../../server/api/capture_settings.go) · [加密持久化](../../../core/messagecapture/repository.go) · [网络代理](references/deployment.md#telegram-网络代理)

## 支持的内容格式

富文本和 Markdown 模式编辑同一份正文，从创建区的「＋ → Markdown 模式」切换。API 和 MCP 将完整 Markdown 放入 `content`。在 Markdown 模式用纯文本粘贴可保留原写法，富文本粘贴会转换标记。 [编辑器说明](../../../web/src/components/MemoEditor/README.md#formatting-and-source-mode)

| 入口 | 可用格式 |
| --- | --- |
| 底部快捷按钮 | 标签、粗体、斜体、无序列表、有序列表、代码块。 |
| 底部「更多格式」 | 正文、一级至三级标题、删除线、行内代码、引用、任务列表、表格、分割线和网页链接。 |
| 富文本 `/` 菜单 | 还可插入四级标题、提示块和可折叠章节。 |
| Markdown 结构 | `#` 至 `######` 标题、`>` 引用、嵌套有序和无序列表、`- [ ]` 与 `- [x]` 任务、带列对齐的 GFM 管道表格、`---` 分割线。单个换行会显示为换行。 |
| Markdown 行内和代码 | `**粗体**`、`*斜体*`、`~~删除线~~`、行内代码、带语言标记的围栏代码、链接、站内锚点、图片、`#标签/子标签` 和 `@用户名`。 |
| Markdown 扩展 | GitHub 风格提示块 `> [!NOTE]`、`> [!TIP]`、`> [!IMPORTANT]`、`> [!WARNING]`、`> [!CAUTION]`，后续行以 `>` 开头。支持脚注、KaTeX 行内 `$…$` 及独立 `$$`、`mermaid` 围栏，以及允许列表中的 `<details>`、`<summary>`、`<kbd>`、`<sub>`、`<sup>` 等 HTML。 |
| 附件与图片 | PNG、JPEG、GIF、WebP 可预览。音视频播放依浏览器支持，PDF 可由浏览器打开，Word、Excel 等普通附件可下载。笔记可导出为 PNG 图片。 |

格式判断以仓库中的解析器、渲染器和允许列表为准。 [解析配置](../../../web/src/lib/memo-ast.ts) · [渲染扩展](../../../web/src/lib/memo-markdown.ts) · [HTML 允许列表](../../../web/src/lib/html-sanitize-schema.ts)

## 验证改动

1. 在新建 SQLite 数据库或独立容器卷按实际入口验证新建、编辑、草稿恢复、搜索、回收站、附件及当前 ZIP 导出再导入。涉及存储格式时验证重启回读和不兼容数据库拒绝启动。 [SQLite 验证命令](../../../store/test/README.md)
2. 覆盖匿名请求、当前账号、过期与撤销凭据。用实际网页 Connect、原生 HTTP 或 MCP 路径验证写入和可见性。检查 MCP 工具发现、只读拒绝写入、分页和上海日期边界。 [API 用例](../../../server/test/) · [MCP 用例](../../../server/mcp/)
3. 收件和代理改动验证设置保存、重启后还原、渠道停用、允许和拒绝的发件人、重复投递、成功确认及代理的独立生效。更新对应功能和接口说明，在总体开发 Skill 记录改动与验证结果。 [收件运行时](../../../server/message_capture.go) · [总体开发记录](../cat-two-development/SKILL.md#开发记录)
