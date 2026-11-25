---
name: cat-two-development
description: Cat_two 整体项目开发指引。用于判断项目定位、确定改动范围、选择验证方式，以及维护仓库文档、NAS 部署、SQLite 架构和 Docker 发布。
---

# Cat_two 项目开发

## 项目定位

Cat_two 是单账户、自托管的个人笔记应用。新实例首次设置时创建一个管理员账户，日常围绕私有笔记的记录、查找、整理和同步展开。桌面 Web 是界面设计与视觉验收目标。Cat_two 使用单账户 SQLite 架构，启动时拒绝不兼容的旧库。通过网页登录记录和管理私有笔记。外部自动化走专用 MCP，消息收件支持 Telegram 和企业微信。

项目基于 Memos，按 Cat_two 自身需求演进。参考其他项目之前，先确认场景，再从本仓库的实际入口、API、权限和数据流找可复用的实现。涉及界面、API 或存储的改动沿用户完成任务的完整链路验证。

## 文档入口

| 查阅主题 | 入口 |
| --- | --- |
| 产品功能、笔记与附件的数据流及内容格式、网页接口、MCP 和消息收件 | [产品功能开发 Skill](../cat-two-product-development/SKILL.md) |
| 桌面布局、主题、组件、交互与浏览器验收 | [UI 设计与开发 Skill](../cat-two-ui-development/SKILL.md) |
| 连接已有实例及使用 Codex 整理笔记 | [MCP 使用 Skill](../cat-two-mcp/SKILL.md) |
| 仓库编码约定、启动命令、生成和验证 | [AGENTS.md](../../../AGENTS.md) |
| 安装、NAS 部署、数据备份与升级、两步验证恢复 | [快速部署](../../../README.md#部署服务端) · [部署与运维](../cat-two-product-development/references/deployment.md) |
| 本地构建、镜像发布和验收 | [构建与发布](references/container-release.md) |
| 各阶段改动、测试与镜像发布证据 | [开发复盘](references/development-log.md) |
| 后续方向、优先级和验收标准 | [优化方案](references/optimization-plan.md) |
| 登录、笔记写入、MCP 和内部实现的审计 | [2026-11-22 代码审计](references/code-audit-2026-11-22.md) |
| 历史设计和 ZIP JSON Schema | [设计归档](references/design/README.md) |

## 开发规范

1. 确认可见需求、目标分支与现有工作区改动。区分源码、运行镜像和部署实例，使用网页 Connect `InstanceService.GetInstanceProfile` 返回的 `version` 和 `commit` 核对部署版本。 [版本检查](../cat-two-product-development/references/external-api.md#凭据和可见性)
2. 按现有层次复用实现。入口从 `cmd` 到 `server`，业务规则放在 `core`，持久化由 `store` 维护，通用工具放在 `internal`。API、数据模型和生成物的修改遵循 [仓库分层](../../../AGENTS.md#repository-layout) 和 [数据库与 Proto 约定](../../../AGENTS.md#database-and-proto-rules)。
3. 回归既有调用与数据。使用隔离实例和临时数据卷验证写入、删除、恢复和重启回读。涉及公网入口、登录或权限时，覆盖匿名、本人、过期或撤销凭据及当前账户的数据归属。检查 HTTPS、可信代理、真实 IP、限流和 SSE。 [部署安全](../cat-two-product-development/references/deployment.md#在公网部署) · [安全说明](../../../SECURITY.md)
4. 保持文档和实现同步。用户可见行为、格式及接口更新到产品功能 Skill，布局、交互和设计规范更新到 UI Skill。架构、交付或开发流程更新到本 Skill，细节由相应参考文档维护。

## 验证

数据库兼容性和数据库测试仅验证 SQLite。后端和存储改动先运行受影响的包或用例，涉及共享逻辑时扩大验证范围。

| 改动 | 验证方式 |
| --- | --- |
| 服务端、鉴权或数据流 | 先运行受影响的包或用 `-run` 定向验证，按改动补 HTTP、Connect 或 MCP 验收。并发和生命周期改动使用 `-race`。 |
| 存储或架构 | 先运行相关的 SQLite 用例。共享存储或架构改动扩到 `go test ./store/...`，覆盖新装、重启回读和不兼容库拒绝。 [存储测试](../../../store/test/README.md) |
| 桌面网页 | 按改动范围运行 `pnpm lint`、`pnpm test` 和生产构建，在浏览器核对相关流程。 [UI 验收](../cat-two-ui-development/SKILL.md) |
| Proto 和生成物 | 运行仓库规定的 `buf generate`、`buf lint` 和格式检查，核对 Go 和 TypeScript 绑定及 MCP 使用的内部 OpenAPI 输出。 [验证命令](../../../AGENTS.md#commands) |
| 文档 | 运行 `git diff --check`，迁移时核对本地链接、章节锚点和文档覆盖。 |

发布前，同一提交须通过完整的 CI 检查、生产构建及 SQLite 容器的新装和重建验证。跨端行为按各功能的验收说明复测，例如导入导出时核对正文、附件、标签与引用。 [测试命令](../../../AGENTS.md#verification-policy) · [产品功能开发](../cat-two-product-development/SKILL.md)

## 部署、镜像和交付

- 用统一的 `compose.yaml` 验证默认命名卷、NAS 目录绑定、运行身份、`/healthz` 和重建后的笔记及附件。已有旧库部署保留原数据卷和镜像，使用独立卷安装当前版本。 [备份与升级](../cat-two-product-development/references/deployment.md#备份和升级) · [Compose](../../../compose.yaml)
- 容器发布用 Cat_two Container 工作流。普通提交按路径触发检查，前端生产构建随容器验证运行。 [构建与发布](references/container-release.md)
- Docker Hub 版本号按 `0.01` 递增，核对编号、`latest`、`sha-<完整提交 SHA>`，以及 AMD64、ARM64 两种平台。分别以 Git、Actions、注册表和 NAS 实例核实源码、构建、发布及部署状态。 [版本编号](../../../scripts/cat_two_docker_release.py) · [发布复盘](references/development-log.md)
- 冒烟脚本在独立数据卷覆盖 Cat_two 新装、容器重建和数据回读。 [冒烟脚本](../../../scripts/release_smoke_test.sh)

## 开发记录

每次修改代码、配置、文档或 Skill，交付前在本节追加上海时间、改动和验证简记。阶段复盘时将较长说明、测试证据、提交与发布链接归入 [开发复盘](references/development-log.md)。 [历次简记](references/development-log.md#历次开发简记)

- **2026-11-22 17:17**　将开发文档拆为项目开发、产品功能开发和 UI 设计开发三份 Skill。功能与接口参考迁入产品 Skill，界面设计和前端约定迁入 UI Skill，旧开发简记归入复盘。合并 `docs` 索引并移除旧目录，更新仓库入口和 MCP 格式引用。核对本地链接、章节锚点及 Skill 元数据。 [产品开发](../cat-two-product-development/SKILL.md) · [UI 开发](../cat-two-ui-development/SKILL.md)
- **2026-11-22 17:29**　发布 Docker Hub `0.10`，核对编号、`latest` 和提交标签及两种平台。正式 AMD64 镜像通过新装、重建、从 `0.09` 升级和浏览器集成验收。 [发布复盘](references/development-log.md#镜像-010)
- **2026-11-22 17:50**　企业微信支持在通知设置保存带端口的公网回调地址。裸域名默认使用 HTTPS，保留已填写的 HTTP 或 HTTPS 协议与端口。服务端提供规范化地址，局域网地址不再用于复制。SQLite 竞态测试、23 项前端专项测试和生产构建通过，本地 Chromium 与容器验证重启保留、官方格式的签名 GET 验证和 POST 收件。 [配置说明](../cat-two-product-development/references/deployment.md#企业微信自建应用) · [官方回调规范](https://developer.work.weixin.qq.com/document/path/90930)
- **2026-11-22 17:58**　发布 Docker Hub `0.11`，核对编号、`latest` 和提交标签在 AMD64 与 ARM64 的一致性。正式 AMD64 镜像通过新装、重建与从 `0.10` 升级验证，Chromium 复测公网回调设置和企业微信模拟回调。 [发布复盘](references/development-log.md#镜像-011)
- **2026-11-22 18:13**　编辑器增加插入标签入口，侧栏按名称排列，搜索增加「无标签」。标签分支可重命名、仅移除标签或将关联笔记移入回收站。Web、REST 和 MCP 统一分支匹配与计数口径，MCP 增加 `hasTags`。全量 SQLite 存储、API 和 MCP 相关竞态测试、1390 项前端测试及生产构建通过。以本地容器和 Chromium 验证插入、筛选、分支移动、删除和恢复。 [标签说明](../cat-two-product-development/SKILL.md#标签) · [接口](../cat-two-product-development/references/external-api.md#统计和标签)
- **2026-11-22 18:34**　调研 flomo 和开源笔记项目，制定 Cat_two 浅色、深色主题。重排侧栏、笔记、编辑器、搜索和设置。桌面侧栏默认 320px，放宽统计、日历和标签的间距，既有自定义宽度可双击手柄复位。增加主题对比度验证，并用桌面 Chromium、生产构建及隔离 SQLite 容器验证。 [桌面设计与配色](../cat-two-ui-development/references/desktop-visual-design.md)
- **2026-11-22 18:44**　发布 Docker Hub `0.12`，核对编号、`latest`、提交标签及 AMD64 和 ARM64 镜像配置。1431 项前端测试通过，正式 AMD64 镜像通过新装、重建、从 `0.11` 升级及桌面 Chromium 验收。 [发布复盘](references/development-log.md#镜像-012)
- **2026-11-22 20:01**　根据确认的预览将浅色、深色的正文和辅助文字设为纯黑、纯白，沿用原画布配色。72 项主题测试和前端生产构建通过，Chromium 核对标题、正文、侧栏统计及背景色。 [配色表](../cat-two-ui-development/references/desktop-visual-design.md#cat_two-主题)
- **2026-11-22 20:06**　浅色和深色的笔记阅读区默认标签使用确认的 `#5783F7`，深色画布改为 `#121212`。主题、阅读器和编辑器测试与生产构建通过。Chromium 核对两种模式的画布、标签字色和底色，以及自定义标签、编辑器和纸张主题。 [配色表](../cat-two-ui-development/references/desktop-visual-design.md#cat_two-主题)
- **2026-11-22 20:37**　网页会话在 45 分钟没有编辑笔记时到期。SQLite 持久化最近编辑时间，浏览、同步和令牌轮换沿用原期限。SQLite 鉴权相关竞态测试、1442 项前端测试、生产构建及 Docker 新装和从 `0.12` 升级测试通过。Chromium 验证两标签页一起退出、重新登录恢复草稿，以及第 44 分钟的续期和刷新沿用原期限。 [会话说明](../cat-two-product-development/references/deployment.md#两步验证)
- **2026-11-22 21:06**　共享 Web 安装升级到 `pnpm/action-setup@v6.1.1`，构建以 `24.22.3` 为最低版本，并跟进后续 Node.js 24.x 更新。Proto 检查改用 `bufbuild/buf-action@v1.7.0` 安装 CLI，执行 lint 和格式检查。以 Node.js `24.22.3` 实测安装步骤和锁文件安装，1442 项前端测试、生产构建、Buf 检查及 `actionlint` 均通过。 [Web 安装](../../../.github/actions/setup-web/action.yml) · [Node.js 安全公告](https://nodejs.org/en/blog/vulnerability/november-2026-security-releases)
- **2026-11-22 21:24**　深色侧栏、搜索按钮、编辑器、笔记卡片及设置面板统一用 `#121212`。Chromium 核对实际像素和三种主题，日历与设置分组采用实色填充。前端静态检查、113 项相关测试和生产构建通过。 [配色表](../cat-two-ui-development/references/desktop-visual-design.md#cat_two-主题)
- **2026-11-22 21:31**　发布 Docker Hub `0.14`，正式 AMD64 镜像通过从 `0.13` 升级和三种主题的 Chromium 验收。公开 README 集中介绍项目与部署，产品及运维细节转入 `.agents`，项目来源写为 Memos。Compose 默认值及迁移后的文档链接已通过检查。 [发布复盘](references/development-log.md#镜像-014) · [产品文档](../cat-two-product-development/references/product-guide.md)
- **2026-11-22 21:56**　浅色桌面首页的侧栏、账号面板、日历、输入框和笔记卡片改用 `#FFFFFF`。前端静态检查、134 项相关测试和生产构建通过。实际构建的 Chromium 截图与确认预览像素一致，同时核对了回收站侧栏、设置、深色与纸张主题。 [配色表](../cat-two-ui-development/references/desktop-visual-design.md#cat_two-主题)
- **2026-11-22 22:00**　设置导航复用标签选中行的底色、文字和图标样式。静态检查、100 项相关测试和生产构建通过。Chromium 验证浅色、深色和纸张模式的配色及鼠标、键盘切换。 [设置导航](../../../web/src/components/Settings/SettingsContent.tsx)
- **2026-11-22 22:07**　发布 Docker Hub `0.15`，核对编号、`latest`、提交标签和 AMD64、ARM64 两种平台。1444 项前端测试通过，正式 AMD64 镜像通过新装、重建、从 `0.14` 升级及 Chromium 验收。 [发布复盘](references/development-log.md#镜像-015)
- **2026-11-23 21:15**　公开 README 在项目标题旁显示小猫图标，复用仓库内的应用图标。 [README](../../../README.md)
- **2026-11-23 22:55**　按确认预览调整浅色首页画布、桌面侧栏与日历配色。生产构建、静态检查和 151 项相关测试通过，Chromium 核对画布、面板与日期的实际颜色及按日筛选。 [配色表](../cat-two-ui-development/references/desktop-visual-design.md#cat_two-主题)
- **2026-11-23 23:11**　MCP 或 PAT 创建的笔记自动添加 `#MCP连接`。保存时识别正文中的真实标签，幂等重试沿用原请求。Go 静态检查、相关服务端及 Markdown 竞态测试通过，覆盖真实 MCP、REST 和 Connect 入口。 [外部 API](../cat-two-product-development/references/external-api.md#令牌创建笔记)
- **2026-11-23 23:17**　发布 Docker Hub `0.16`，核对编号、`latest`、提交标签和 AMD64、ARM64 两种平台。前后端 CI 通过，正式 AMD64 镜像通过新装、重建、从 `0.15` 升级以及配色与 MCP、PAT 标签的验收。 [发布复盘](references/development-log.md#镜像-016)
- **2026-11-24 21:45**　按确认稿更新浅色首页的纯白画布、侧栏间距和日历色块，当前日期用细描边标记。默认头像复用 Cat_two 小猫图标，已上传的头像优先显示。1448 项前端测试、静态检查和生产构建通过，Chromium 核对配色、头像、日期提示、鼠标和键盘筛选及窄屏布局。 [桌面设计与配色](../cat-two-ui-development/references/desktop-visual-design.md)
- **2026-11-24 21:48**　移除浅色桌面首页日历色块在鼠标悬停和键盘聚焦时的日期提示。色块继续支持按日筛选，辅助技术可读取完整日期。26 项日历测试、静态检查和前端生产构建通过，Chromium 验证浅色和深色的提示行为及鼠标、键盘筛选。 [日历色块](../../../web/src/components/ActivityCalendar/CalendarCell.tsx)
- **2026-11-24 22:29**　按云端审计收紧个人令牌管理，统一改密与会话撤销的事务，修复草稿、任务勾选、缓存和回收站隐私显示。SQLite 启动增加实例锁，提供离线 Markdown 检查与修复，附件清理采用持久重试，MCP 分页限制正文大小。镜像发布等待同提交检查，按摘要验收后推广标签，固定 Proto 生成器并恢复历史文档。Go 全仓、三种兼容数据库定向测试、1458 项前端测试及 Chromium 通过。本地 AMD64 候选通过新装、重建、从 `0.16` 升级、附件和回收站恢复及离线正文修复。 [发布流程](references/container-release.md) · [部署与恢复](../cat-two-product-development/references/deployment.md) · [接口](../cat-two-product-development/references/external-api.md)
- **2026-11-24 22:43**　移除 MySQL、MariaDB 和 PostgreSQL 的驱动、迁移、查询方言及兼容测试。数据库兼容性和测试只覆盖 SQLite，日常先验证受影响的功能。Go 全仓测试、SQLite 迁移与并发专项测试、Proto 和 Compose 检查通过。隔离实例验证了新装、按标签查询，以及重启后读取笔记和附件。 [验证策略](#验证) · [发布流程](references/container-release.md)
- **2026-11-24 23:35**　批量待办复用修订冲突保护。侧栏、编辑器和引导共享本人每日统计，1 万条笔记样本的未压缩响应从两次共 960,560 B 降至一次 1,605 B。删除孤立语音依赖、多人空间和旧归属、SSO、出站 Webhook 及 S3 附件迁移。SQLite 继续保护作者、笔记和本地附件。MCP 启动只解析已发布工具，Web 测试和文档改动跳过镜像构建，本地冒烟隔离前端产物，Go 依赖可跨 runner 缓存并离线编译。 [产品接口](../cat-two-product-development/references/external-api.md) · [升级说明](../cat-two-product-development/references/deployment.md#备份和升级) · [构建与验证](references/container-release.md)
- **2026-11-24 23:57**　外部笔记工具统一使用 MCP 专用令牌，退役通用 REST 和 PAT。网页通过登录会话与 MCP 共用笔记服务和修订检查。首次安装创建唯一管理员，启动时检查 SQLite 结构，误挂不兼容旧库会停止启动。 [连接和权限](../cat-two-product-development/references/external-api.md) · [部署与数据目录](../cat-two-product-development/references/deployment.md#备份和升级)
- **2026-11-25 00:25**　发布 Docker Hub `0.17`，验证 AMD64、ARM64 新装和同卷重建，核对编号、`latest` 和提交标签的镜像摘要。 [发布记录](references/development-log.md#镜像-017)
- **2026-11-25 22:08**　浅色主题的页面画布统一使用纯白色。Chromium 验证首页和回收站在桌面、窄屏与跟随系统浅色时均为 `RGB(255, 255, 255)`，并核对了深色和纸张主题。 [配色表](../cat-two-ui-development/references/desktop-visual-design.md#cat_two-主题)
- **2026-11-25 22:12**　浅色回收站的空状态、加载失败提示框和已删除笔记卡片复用首页的纯白画布色。主题、页面布局和回收站的 85 项定向测试、前端静态检查及生产构建通过。 [配色表](../cat-two-ui-development/references/desktop-visual-design.md#cat_two-主题)
- **2026-11-25 22:15**　浅色首页已保存笔记卡片的默认外边框改为 1px 纯黑色，单列和多列共用此样式。119 项定向测试、前端静态检查及生产构建通过。 [桌面配色](../cat-two-ui-development/references/desktop-visual-design.md#cat_two-主题)
