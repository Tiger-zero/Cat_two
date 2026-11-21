---
name: cat-two-development
description: 开发和维护 Cat_two 单账户自托管笔记。用于评估与移植功能、排查网页和 MCP、验证数据迁移、NAS 升级及 Docker 发布。
---

# Cat_two 开发

## 项目定位与开发原则

Cat_two 面向单人自托管，当前网页版以桌面浏览器为目标。新实例在首次设置时创建一个管理员账户，日常使用围绕自己的私有笔记、整理和同步展开。旧账号、空间和可见性记录属于历史兼容范围。涉及这些数据时，沿用现有所有权与访问校验，并验证升级后的笔记、附件及引用。

项目源自 Fumos 和 Memos，后续按 Cat_two 自身需求独立演进。移植前先确认使用场景，从本仓库 `main` 的界面、API、权限和数据流找已有实现。选用适合单账户的部分，复用本项目的组件、接口与数据模型，沿完整流程优化可合并的逻辑和可减少的查询。

## 参考资料

| 查阅主题 | 入口 |
| --- | --- |
| 当前功能、保存链路、部署数据与集成边界 | [功能与实现](references/product-guide.md) |
| 每日变更、测试证据和镜像发布记录 | [开发复盘](references/development-log.md) |
| 优先级、代码证据和验收方式 | [优化方案](references/optimization-plan.md) |
| 上游设计、历史约定及 ZIP JSON Schema | [设计参考](references/design/README.md) |
| 编码约定、启动和静态检查 | [仓库开发规范](../../../AGENTS.md) |
| NAS 配置、备份升级和两步验证恢复命令 | [项目 README](../../../README.md) |
| Codex 接入及笔记归档工作流 | [MCP Skill](../cat-two-mcp/SKILL.md) |

## 开发与验证

1. 确认目标提交、工作区与部署版本。仓库源码描述的是拟交付功能，NAS 实际运行版本可查 `/api/instance/profile` 的 `version` 和 `commit`。在隔离数据库或容器卷测试写入、删除、恢复和迁移。
2. 界面从 [页面路由](../../../web/src/router/index.tsx)、[编辑器](../../../web/src/components/MemoEditor/README.md) 和 [查询 hooks](../../../web/src/hooks/) 入手。界面设计、实现和视觉验收以桌面 Web 为范围，用现有设计系统核对简中、英语、浅色和深色。统一按 `Asia/Shanghai` 处理日历和按日筛选。网页改动按需运行 `pnpm lint`、`pnpm test` 与 `pnpm build`。
3. 服务端从 [API](../../../server/api/)、[业务规则](../../../core/) 追到 [Store](../../../store/doc.go) 和 [数据模型](../../../proto/)。按 Cat_two 单账户 SQLite 部署验证新建数据库、增量迁移和 CalVer 升级用例。API 定义修改走生成流程。 [存储测试命令](../../../store/test/README.md)
4. 访问控制覆盖账户本人、匿名请求及隔离测试中的旧账号数据。改动登录、附件或 MCP 时，核对令牌或会话过期、撤销后下一次请求、数据归属及既有内容读取。服务端可按改动范围运行 `go test -race ./server/...` 及对应存储测试。
5. 按实际入口走一遍关联流程。笔记编辑核对日历到保存、刷新后显示及附件下载。导入导出核对 ZIP 内的正文、附件、标签和引用。MCP 核对工具发现、分页、上海日期边界和调用范围。改变用户可见行为时更新功能说明及相应验收项。

## 镜像和交付

- 用户提出发布镜像时，按 [Cat_two Container](../../../.github/workflows/cat-two-container.yml) 工作流执行。Docker Hub 版本号按 `0.01` 递增，核对版本号、`latest` 和 `sha-<完整提交 SHA>`，以及 AMD64、ARM64 两种平台。 [编号实现](../../../scripts/cat_two_docker_release.py)
- 普通提交按改动路径触发后端、前端、Proto 和容器检查。前端生产构建随容器验证运行。在 `main` 手动发布或通过 `[publish dockerhub]` 发布时执行完整构建。 [工作流](../../../.github/workflows/)
- 分别用 Git、Actions、注册表和 NAS 实例证据确认源码推送、构建验证、镜像发布与实际升级。部署改动核对两份 Compose、`/healthz`、重建前后数据和附件可读写。历史发布证据见 [开发复盘](references/development-log.md)。
- 冒烟脚本默认验证 Cat_two 新装及重建后的数据。用 `--previous-image` 指定实际的旧版 Cat_two 镜像，可在隔离卷中验证升级。 [冒烟脚本](../../../scripts/release_smoke_test.sh)

## 支持的内容格式

以下清单以仓库当前源码为准。笔记正文以 Markdown 保存，富文本和 Markdown 模式是同一篇笔记的两个编辑界面。通过底部「＋ → Markdown 模式」切换。 [编辑器说明](../../../web/src/components/MemoEditor/README.md#formatting-and-source-mode)

| 入口 | 可用格式 |
| --- | --- |
| 底部快捷按钮 | 粗体、斜体、无序列表、有序列表、代码块。 |
| 底部「更多格式」 | 正文、一级至三级标题、删除线、行内代码、引用、任务列表、表格、分割线和网页链接。 |
| 富文本 `/` 菜单 | 还可插入四级标题、提示块和可折叠章节。 |

### Markdown 和同步

- **结构**　`#` 至 `######` 标题、`>` 引用、无序和有序列表、嵌套列表、`- [ ]` 与 `- [x]` 任务、带列对齐的 GFM 管道表格、`---` 分隔线。单个换行会在正文显示为换行。
- **行内和代码**　粗体 `**文字**`、斜体 `*文字*`、删除线 `~~文字~~`、用反引号包住的行内代码、围栏代码块（可标注语言）、普通链接、站内锚点、Markdown 图片、`#标签/子标签` 和 `@用户名`。
- **扩展**　GitHub 风格提示块使用独立的首行 `> [!NOTE]`、`> [!TIP]`、`> [!IMPORTANT]`、`> [!WARNING]` 或 `> [!CAUTION]`，正文续行以 `>` 开头。脚注用 `[^1]` 和 `[^1]: 说明`。KaTeX 行内公式用 `$…$`，独立公式的开头和结尾各用单独一行 `$$`。Mermaid 图使用标记为 `mermaid` 的代码围栏。HTML 按允许列表处理，常用例子有 `<details>`、`<summary>`、`<kbd>`、`<sub>` 和 `<sup>`。 [解析配置](../../../web/src/lib/memo-ast.ts) · [渲染扩展](../../../web/src/lib/memo-markdown.ts) · [HTML 允许列表](../../../web/src/lib/html-sanitize-schema.ts)
- **同步和粘贴**　通过 MCP 或 API 把完整 Markdown 作为笔记 `content` 提交。手动粘贴时，用 Markdown 模式里的纯文本粘贴保留原写法。富文本粘贴会将格式转换成 Markdown，可能规范化标记的写法。 [同步入口](../../../server/mcp/README.md#tool-surface)

### 附件和归档

| 用途 | 常用格式和行为 |
| --- | --- |
| 笔记附件 | 常见图片如 PNG、JPEG、GIF、WebP 可预览。视频和音频的播放取决于浏览器支持。PDF（`.pdf`）可交由浏览器打开，Word（`.doc`、`.docx`）、Excel（`.xls`、`.xlsx`）及其他普通附件可按原文件名下载。回形针是通用上传入口。 [附件列表](../../../web/src/components/MemoMetadata/Attachment/AttachmentListView.tsx) · [文件类型识别](../../../server/api/attachment_service.go) |
| 笔记导入和导出 | 批量迁移使用项目 ZIP 归档（`memos-export` 1.x），包含 `manifest.json`、每篇笔记的 `.md` 正文与 `.json` 元数据，以及附件或附件外链。单篇 Markdown 可通过编辑器粘贴或走 API/MCP 同步。 [归档规范](../../../core/memoexport/format.go) |
| 笔记图片 | 可把一条笔记导出为 PNG 图片。 [图片导出](../../../web/src/components/MemoActionMenu/MemoShareImageDialog.tsx) |

## 开发记录

每次修改代码、配置、文档或 Skill，交付前在这里补一条上海时间、改动和验证的简记。较长的过程、测试与发布证据可追加到 [开发复盘](references/development-log.md)，保留可追溯的提交或发布链接。阶段复盘时把已完成事项归入对应日期。

- **2026-11-21**　汇总单账户收敛、笔记编辑和附件、MCP 接入、存储验证及镜像进展。 [当日复盘](references/development-log.md)
- **2026-11-21 22:00**　重写开发 Skill，精简功能说明并迁移历史设计。文档链接、ZIP Schema 字节比对和归档、配置、限流测试通过。 [优化方案](references/optimization-plan.md)
- **2026-11-21 22:20**　完成笔记时间、草稿、归档导入和回收站清理修复。搜索支持归档范围，SQLite 默认列表和 MCP 标签读取已优化。 [P1、P2 修复与验证](references/development-log.md#p1-与-p2-修复)
- **2026-11-21 22:31**　将本地默认存储测试、后端 CI 和升级验证统一到 SQLite。默认存储全量、单项迁移筛选和工作流语法检查通过。 [存储测试命令](../../../store/test/README.md)
- **2026-11-21 22:46**　按改动路径运行日常 CI，合并前端生产构建，移除 Codecov 和上游发布工作流。Stale 和旧版 Memos 升级改为手动运行。SQLite 全套、旧版升级、启动脚本和容器触发条件验证通过。 [工作流](../../../.github/workflows/) · [容器 CI](https://github.com/Tiger-zero/Cat_two/actions/runs/44915807863)
- **2026-11-21 23:00**　关闭 Stale 及旧版 Memos 升级工作流，清理上游镜像测试和本地冒烟的自动旧镜像选择。SQLite 存储全套和 Cat_two 新装、重建及从 `0.06` 升级验证通过。 [存储测试](../../../store/test/README.md) · [冒烟脚本](../../../scripts/release_smoke_test.sh)
- **2026-11-21 23:15**　首页侧栏复用日历查询，展示未归档笔记、不同标签路径和上海时区的记录天数。调整树形及平铺标签颜色、置顶区分隔和子标签引导线，深色侧栏使用黑底白字。浏览器核对桌面和窄屏、跨午夜计数、保存后刷新和标签筛选与置顶。 [功能说明](references/product-guide.md#网页里的功能)
- **2026-11-21 23:19**　将后续界面设计、实现和视觉验收范围明确为桌面 Web，核对开发规范和产品定位表述一致。 [功能说明](references/product-guide.md)
- **2026-11-21 23:30**　调整桌面笔记页间距，默认侧栏宽 280px，主要区块间距 24px，视图和标签行高 32px，单列编辑器和笔记卡片间距 16px。核对浅色、深色、侧栏宽度保存、标签筛选和多列布局。前端静态检查、1,379 项测试和生产构建通过。 [功能说明](references/product-guide.md#网页里的功能)
- **2026-11-21 23:50**　桌面首页标题和新笔记输入框随列表滚动吸顶。单列和多列共用同一编辑器，书写时展开并用 `RGB(48, 207, 121)` 画边框，离开时收起。浏览器核对浅色、深色、长草稿内滚动、切换布局保留附件和从列表中段保存。前端静态检查、全量测试和生产构建通过。 [功能说明](references/product-guide.md#网页里的功能)
- **2026-11-22 00:03**　让桌面左侧看板独立滚动，标签笔记数随鼠标进入标签区或键盘聚焦显示。回收站接在最后一个标签下方，沿用 4px 行间距，移除分隔线。用 48 个标签核对滚动边界、计数显示、树形和平铺行间距、滚动条与宽度拖动。前端静态检查、全量测试和生产构建通过。 [功能说明](references/product-guide.md#网页里的功能)
- **2026-11-22 00:09**　按确认的预览将首页「全部笔记」标题改为 17px、字重 700。浏览器核对浅色和深色的实际样式及标题菜单开合。前端静态检查、29 项相关测试和生产构建通过。 [标题菜单](../../../web/src/components/MemoFeedMenu.tsx)
- **2026-11-22 00:20**　移除侧栏「新建备忘录」及其独立弹窗，从首页右侧吸顶输入框新建笔记。重新打开首页时优先恢复首页草稿，同账号的旧侧栏草稿和待上传附件也可恢复。浏览器核对草稿恢复顺序、滚动中保存及编辑器聚焦模式。前端静态检查、1,359 项测试和生产构建通过。 [功能说明](references/product-guide.md#网页里的功能)
- **2026-11-22 00:28**　发布 Docker Hub `0.07`，核对编号、`latest`、提交标签及 AMD64 和 ARM64 两种平台。用正式镜像和独立 SQLite 卷验证新装、容器重建以及从 `0.06` 升级后的笔记读写。 [发布记录](references/development-log.md#2026-11-22上海时间)
- **2026-11-22 00:42**　将日历「视图选项」浮层改为按视口定位，消除两列笔记滚动时的瞬时位移，保留左侧独立滚动时的锚点跟踪。用 `0.07` 和修复后源码对照，检查滚轮事件和逐帧坐标、左栏滚动、排序选项及 Esc 返回焦点。57 项相关测试、前端静态检查和生产构建通过。 [日历视图选项](../../../web/src/components/MemoDisplaySettingMenu.tsx)
- **2026-11-22 00:56**　将设置移到笔记页上方的独立弹窗，左侧选择分区，右侧单独滚动。旧 `/setting#分区` 链接登录后在首页打开对应分区。桌面浅色和深色核对草稿、筛选和笔记滚动位置、嵌套的 MCP 弹窗及关闭后的焦点。前端静态检查、1,361 项测试和生产构建通过。 [设置弹窗](../../../web/src/components/Settings/SettingsDialog.tsx)
- **2026-11-22 01:13**　回收站保留进入前的日历和标签，点日期或标签回到对应列表。选中标签及回收站行统一用 `RGB(57, 115, 84)`。移除保存视图的界面、REST、Connect 和 MCP 工具，旧 `/views` 链接跳到首页，旧 SQLite 记录随升级保留。浅色和深色桌面验证侧栏滚动、标签选择、恢复笔记及归档返回。前端静态检查、1,342 项测试、生产构建及 SQLite 和服务端回归验证通过。 [功能说明](references/product-guide.md#网页里的功能) · [API 变更](../../../proto/api/CHANGELOG.md)
- **2026-11-22 01:21**　核对 Docker Hub `0.08`、`latest` 和提交标签及 AMD64、ARM64 两种平台。正式镜像通过从 `0.07` 升级的独立 SQLite 测试，旧笔记可读、新笔记可写，旧视图记录保留。Chromium 核对设置弹窗、回收站侧栏和标签颜色。 [发布记录](references/development-log.md#2026-11-22上海时间)
