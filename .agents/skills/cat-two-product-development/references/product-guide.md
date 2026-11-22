# Cat_two 功能与实现

Cat_two 是面向单账户使用的自托管笔记应用，通过 Docker 部署，在桌面浏览器中使用，默认用 SQLite。旧版本遗留的笔记、附件和关联数据按原所有者保留。此页按仓库当前源码记录产品行为。核对在用镜像时，读取实例 `/api/instance/profile` 的 `version` 和 `commit`，再查看镜像标签对应的源码。 [部署与镜像说明](../../../../README.md)

## 网页里的功能

- 「设置 → 安全 → 黑名单 IP」供管理员查看连续输错密码后被禁止登录的 IP、封禁时间及解除入口。封禁随 SQLite 保存。 [登录规则和主机解封](../../../../README.md#登录防护和-ip-解封)

| 功能 | 当前用法 | 代码入口 |
| --- | --- | --- |
| 登录和安全 | 首次启动按页面引导创建管理员，随后关闭网页注册。在「系统」修改密码，在「我的账户」启用验证器两步验证。丢失验证器时按 README 从运行中的容器恢复。 | [登录与会话](external-api.md#凭据和可见性) · [账户服务](../../../../server/api/user_service.go) · [恢复步骤](../../../../README.md#重置两步验证) |
| 日历和笔记列表 | 「日历」打开笔记列表。点侧栏日期筛选当天，选中日期后新笔记带上该日，点筛选标签的「×」清除。同一天更改搜索或标签时，手动设置的笔记时间会保留。日历、时间显示和日期筛选用 `Asia/Shanghai`。旧按日链接会转到当前列表。 | [首页](../../../../web/src/pages/Home.tsx) · [日期工具](../../../../web/src/lib/calendar-utils.ts) · [旧日历链接](../../../../web/src/router/LegacyCalendarRedirect.tsx) |
| 侧栏统计 | 首页日历上方显示当前账户的未归档笔记数、不同的标签路径数和记录天数。记录天数按上海时区的笔记创建日期去重。统计复用侧栏查询，随笔记变更更新，独立于当前筛选和日历排序设置。 | [统计查询](../../../../web/src/hooks/useFilteredMemoStats.ts) · [统计接口](external-api.md#统计和标签) |
| 搜索和筛选 | 登录后可在搜索面板选择「未归档」或「归档」。在两个列表间切换搜索范围时，日期和标签会一起保留。可用关键词或 CEL 搜索。「全部笔记」可按创建时间或更新时间排序、筛选来源及多选导出。网页创建记为「网页端」，旧笔记缺少来源时显示「未记录」。旧 `/views` 链接会打开笔记列表。 | [搜索](../../../../web/src/components/AppSidebar/QuickFindDialog.tsx) · [列表菜单](../../../../web/src/components/MemoFeedMenu.tsx) · [CEL](../../../../filter/README.md) |
| 标签 | 侧栏按路径展示标签，选择父标签会包含子路径。鼠标进入桌面标签区或用键盘聚焦区内控件时，显示各标签的笔记数。重命名会带动下级路径，目标已存在时合并。置顶按账户在当前浏览器保存。「设置 → 标签」可整理斜杠两侧的空格。 | [标签侧栏](../../../../web/src/components/AppSidebar/TagsSection.tsx) · [标签接口](external-api.md#统计和标签) |
| 编辑和草稿 | 从首页右侧输入框新建笔记。富文本和 Markdown 源码共用格式操作，从「＋ → Markdown 模式」切换。草稿和待上传附件保存在当前浏览器。重新打开首页且首页草稿为空时，可恢复同一账号在旧侧栏入口留下的草稿。打开已有笔记时，符合当前版本的编辑草稿可恢复。遇到版本冲突，编辑器会提示。 | [首页](../../../../web/src/pages/Home.tsx) · [编辑器与格式](../../../../web/src/components/MemoEditor/README.md) · [保存逻辑](../../../../web/src/components/MemoEditor/edit/hooks/useMemoSave.ts) |
| 附件 | 用回形针添加图片、PDF、Word、Excel 等文件。可以只添加附件并保存。保存后可预览图片或下载文档，部分图片可涂画。站内下载使用当前网页会话，遇到错误时页面会显示状态。 | [附件组件](../../../../web/src/components/MemoMetadata/Attachment/) · [上传](../../../../server/api/attachment_upload.go) · [下载](../../../../web/src/lib/attachment-download.ts) |
| 私有访问和归档 | 笔记、统计及附件只对所属账号开放，旧笔记和旧空间数据沿用作者权限。详情支持笔记引用、评论、反应和导出为图片。可置顶笔记，或在归档列表查阅已归档的笔记。 | [访问规则](../../../../core/access/memo.go) · [详情](../../../../web/src/pages/MemoDetail.tsx) · [归档](../../../../web/src/pages/Archived.tsx) |
| 回收站 | 进入回收站后，右侧显示已删除的笔记，左侧保留原列表的日历和标签。点击日期或标签可以回到对应列表并筛选。创建者可恢复笔记或彻底删除。恢复带回原状态和附件。超过 30 天后由服务端定期清理。彻底删除以数据库提交结果为准，文件清理失败会由后台任务重试。 | [回收站页面](../../../../web/src/pages/Trash.tsx) · [回收站接口](../../../../server/api/memo_trash_handler.go) · [保留规则](../../../../store/memo_trash.go) |
| 设置 | 从账号菜单打开独立设置弹窗。左侧选择分区，右侧滚动查看设置。点右上角「×」或按 Esc 回到原笔记位置，筛选和草稿会保留。旧版 `/setting#分区` 链接在首页打开对应分区。默认简体中文，可切换英语。主题可跟随系统，也可选浅色、深色和纸张。可配置账号资料、媒体元数据和编辑偏好。受支持的手机浏览器可把网页加到主屏幕，默认使用小猫图标。 | [设置弹窗](../../../../web/src/components/Settings/SettingsDialog.tsx) · [设置项](../../../../web/src/components/Settings/settingSections.ts) · [语言](../../../../web/src/i18n.ts) · [主题](../../../../web/src/themes/) · [网页应用配置](../../../../web/public/site.webmanifest) |

桌面布局、编辑器吸顶与聚焦状态、侧栏和标签样式、配色及字体见 UI 开发 Skill。部分旧资料引用的独立日历、附件库和地图已从网页移除，旧笔记的位置元数据可通过 ZIP 保存。 [桌面 UI 开发](../../cat-two-ui-development/SKILL.md) · [API 变更记录](../../../../proto/api/CHANGELOG.md)

将其他工具生成的总结同步到 Cat_two 时，把原始 Markdown 放入 API 的 `content`。在网页手动录入时，可用 Markdown 模式纯文本粘贴。 [Markdown 与附件格式](../SKILL.md#支持的内容格式)

## 前端、服务端和数据库

```mermaid
flowchart LR
  Web["网页与编辑器"] --> Client["React Query 和 Connect"]
  Client --> API["Go API 与作者权限"]
  MCP["MCP 客户端"] --> Access["连接令牌"]
  Access --> API
  Access --> Personal["按作者查询"]
  Personal --> Store["Store"]
  API --> Store
  Store --> SQLite["SQLite 元数据与查询"]
  Store --> Markdown["本地 Markdown 正文"]
  API --> Files["本地附件"]
  API --> SSE["SSE 变更通知"]
  SSE --> Client
```

- 浏览器以 React、TypeScript 和 Vite 实现。路由及上下文携带筛选和显示偏好，React Query 管理读取和缓存。[路由](../../../../web/src/router/index.tsx) · [上下文](../../../../web/src/contexts/) · [请求 Hooks](../../../../web/src/hooks/)
- Go 服务执行数据库迁移并提供嵌入网页、Connect、REST、文件服务、MCP 和 `/healthz`。网页先上传附件，再提交 Markdown、附件引用和时间。写入时后端记录作者、刷新标签和正文属性。编辑提交字段掩码，SSE 通知其他页面重新取数。 [启动入口](../../../../cmd/memos/main.go) · [保存逻辑](../../../../web/src/components/MemoEditor/edit/services/memoService.ts) · [笔记服务](../../../../server/api/memo_service.go) · [实时刷新](../../../../web/src/hooks/useLiveMemoRefresh.ts)
- 新建草稿在保存前记录创建 ID 和提交内容，响应中断后可重试原请求。编辑按笔记修订号提交，冲突会保留草稿，供用户载入当前笔记并核对。ZIP 导入新建时一并提交正文、附件、置顶和归档状态。 [草稿提交](../../../../web/src/components/MemoEditor/edit/hooks/useMemoSave.ts) · [原子写入](../../../../server/api/user_service_memo_import.go)
- Store 负责和数据库驱动交互。Compose 默认使用 SQLite，生产数据库名为 `memos_prod.db`，使用 WAL。服务端也有 MySQL 和 PostgreSQL 驱动。[数据层](../../../../store/doc.go) · [驱动](../../../../store/db/db.go) · [SQLite](../../../../store/db/sqlite/sqlite.go)

## 数据、备份和升级

- Docker 和 NAS 共用一份 Compose，默认将 `cat_two_data` 命名卷挂载到 `/var/opt/memos`。NAS 可以直接替换服务的挂载源，指向宿主机绝对路径。升级时备份实际数据源、保留项目和挂载，并确保运行身份可读写。 [Compose 配置](../../../../compose.yaml) · [备份和升级](../../../../README.md#备份和升级)
- 「设置 → 存储」集中显示数据目录和使用量。笔记正文保存在 `notes/` 的 Markdown 文件，新附件保存在 `assets/`。SQLite 管理账户、笔记元数据、标签、引用和搜索索引。通过应用、REST 或 MCP 修改内容，由应用一并更新文件和索引。 [存储设置](../../../../web/src/components/Settings/StorageSection.tsx)
- 完整实例备份使用一致性快照，或先停止服务再复制数据目录和部署配置。同一次备份的 SQLite 和文件目录一起恢复。旧 SQLite 附件和 S3 对象在升级时写入 `assets/`，原本地附件继续从原路径读取。升级前也备份独立目录和旧对象存储，保留 S3 读取凭据直到迁移成功。用备份副本和独立 Compose 项目验证恢复。 [恢复操作](../../../../README.md#备份和升级)
- 多选可导出最多 100 条笔记，也可在「设置 → 导出和导入」导出整个账户的笔记。ZIP 含 Markdown、笔记元数据和可归档附件。导入时先预演，可保留现有、覆盖或创建副本。完成后根据逐项报告检查附件、引用、评论和归档状态。[导入导出](../../../../web/src/components/Settings/MemoExportSection.tsx) · [归档格式](../../../../core/memoexport/format.go)
- ZIP 导入可恢复同一作者的归档笔记及评论层级。新导出的 ZIP 原样存储附件数据，易压缩文件较多时归档可能变大。导入仍会检查解压大小和压缩比，已经超出限制的旧 ZIP 仍需从原实例重新导出。覆盖导入时，归档未携带的笔记引用仍保留。 [评论规则](../../../../store/memo_comment.go) · [ZIP 读写](../../../../core/memoexport/) · [导入实现](../../../../server/api/user_service_memo_import.go)

## 外部集成

- 对外 REST 路由、MCP 工具参数、凭据的权限、分页和写回 Markdown 的约定集中维护在外部接口参考。 [外部接口](external-api.md) · [公网部署](../../../../README.md#在公网部署)
- 「设置 → 通知」可分别启用 Telegram Bot 私聊和企业微信自建应用，接收指定发件人的文本。消息写成管理员的私有笔记并自动标记 `#Telegram` 或 `#微信`，保存成功后原会话收到 `Yes!`。通过「设置 → 网络」可为 Telegram 轮询和回复指定代理。保存设置后立即生效，配置随数据卷保留。 [配置说明](../../../../README.md#telegram-和企业微信收件) · [代理说明](../../../../README.md#telegram-网络代理) · [笔记写入](../../../../server/api/memo_capture.go)

在「设置 → MCP 和 API」管理连接。普通 PAT 用于 REST。专用 MCP 令牌创建后可通过 Streamable HTTP 调用 `/mcp`，有效期为 7 天（默认）、14 天、1 个日历年或永久。令牌可选只读或读写，默认读写。到期或删除连接后，下一次请求被拒绝。设置会显示权限、首次连接的客户端 IP 和令牌到期时间。 [API](../../../../proto/api/README.md) · [接入 Codex](../../../../plugins/cat-two-notes/README.md#连接-nas)

| MCP 工作流 | 工具 |
| --- | --- |
| 核对账户 | `auth_get_current_user` |
| 查询笔记 | `memo_search` 按字面关键词、标签、创建日、是否置顶等条件筛选活跃笔记。日期用上海时区并包含首尾。按 CEL 和归档状态查询可用 `memo_list_memos`。 |
| 读取正文和附件 | `memo_batch_get` 可按名称读取多条自己的笔记及已归档笔记正文。附件详情可用 `memo_get_memo`。 |
| 发现标签 | `tag_tree` 和 `tag_search` 按路径返回本人活跃笔记的标签，父路径的数量包含下级笔记。 |
| 创建或更新 | 用 `memo_create_memo` 或 `memo_update_memo` 写 Markdown。笔记插件在保存前预览并等用户确认。 |

旧镜像可用工具发现结果选择原有查询和逐条读取工具。参数、结果分页和兼容用法见工具说明。[MCP 参数与范围](../../../../server/mcp/README.md#tool-surface) · [笔记工作流](../../../../plugins/cat-two-notes/skills/organize-notes/SKILL.md)

开发、测试和镜像发布流程见总体开发 Skill 和 README。 [总体开发](../../cat-two-development/SKILL.md) · [构建和发布镜像](../../../../README.md#构建和发布镜像)
