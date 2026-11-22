---
name: cat-two-development
description: Cat_two 整体项目开发指引。用于判断项目定位、确定改动范围、选择验证方式，以及维护仓库文档、NAS 部署、数据库迁移和 Docker 发布。
---

# Cat_two 项目开发

## 项目定位

Cat_two 是单账户、自托管的个人笔记应用。新实例首次设置时创建一个管理员账户，日常围绕私有笔记的记录、查找、整理和同步展开。桌面 Web 是界面设计与视觉验收目标，NAS 和 Docker 部署使用 SQLite。处理旧实例时要保留原有笔记、附件、引用和归属校验。

项目基于 Memos，按 Cat_two 自身需求演进。参考其他项目之前，先确认场景，再从本仓库的实际入口、API、权限和数据流找可复用的实现。涉及界面、API 或存储的改动沿用户完成任务的完整链路验证。

## 文档入口

| 查阅主题 | 入口 |
| --- | --- |
| 产品功能、笔记与附件的数据流、内容格式、REST 和 MCP | [产品功能开发 Skill](../cat-two-product-development/SKILL.md) |
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

1. 确认可见需求、目标分支与现有工作区改动。区分源码、运行镜像和部署实例，使用 `/api/instance/profile` 返回的 `version` 和 `commit` 核对部署版本。
2. 按现有层次复用实现。入口从 `cmd` 到 `server`，业务规则放在 `core`，持久化由 `store` 维护，通用工具放在 `internal`。API、数据模型和生成物的修改遵循 [仓库分层](../../../AGENTS.md#repository-layout) 和 [数据库与 Proto 约定](../../../AGENTS.md#database-and-proto-rules)。
3. 回归既有调用与数据。使用隔离实例和临时数据卷验证写入、删除、恢复及迁移。涉及公网入口、登录或权限时，覆盖匿名、本人、过期或撤销凭据及既有数据归属。检查 HTTPS、可信代理、真实 IP、限流和 SSE。 [部署安全](../cat-two-product-development/references/deployment.md#在公网部署) · [安全说明](../../../SECURITY.md)
4. 保持文档和实现同步。用户可见行为、格式及接口更新到产品功能 Skill，布局、交互和设计规范更新到 UI Skill。架构、交付或开发流程更新到本 Skill，细节由相应参考文档维护。

## 验证

| 改动 | 验证方式 |
| --- | --- |
| 服务端、鉴权或数据流 | 按改动范围运行 `go test -race ./server/... ./core/...`，从实际 HTTP、Connect 或 MCP 入口复测。 |
| 存储或迁移 | 运行 `go test ./store/...`，覆盖 SQLite 新装、增量迁移和 CalVer 升级。按明确的兼容任务运行其他数据库测试。 [存储测试](../../../store/test/README.md) |
| 桌面网页 | 按改动范围运行 `pnpm lint`、`pnpm test` 和生产构建，在浏览器核对相关流程。 [UI 验收](../cat-two-ui-development/SKILL.md) |
| Proto 和生成物 | 运行仓库规定的 `buf generate`、`buf lint` 和格式检查，核对 Go、OpenAPI 和 TypeScript 输出。 [验证命令](../../../AGENTS.md#commands) |
| 文档 | 运行 `git diff --check`，迁移时核对本地链接、章节锚点和文档覆盖。 |

跨端行为按各功能的验收说明复测。例如编辑后刷新及下载附件，导入导出时核对正文、附件、标签与引用，MCP 检查发现、分页、上海日期边界和调用权限。 [产品功能开发](../cat-two-product-development/SKILL.md)

## 部署、镜像和交付

- 用统一的 `compose.yaml` 验证默认命名卷、NAS 目录绑定、运行身份、`/healthz` 和重建后的笔记及附件。切换已有部署先核对生效的镜像、挂载、端口和运行身份。 [备份与升级](../cat-two-product-development/references/deployment.md#备份和升级) · [Compose](../../../compose.yaml)
- 容器发布用 Cat_two Container 工作流。普通提交按路径触发检查，前端生产构建随容器验证运行。 [构建与发布](references/container-release.md)
- Docker Hub 版本号按 `0.01` 递增，核对编号、`latest`、`sha-<完整提交 SHA>`，以及 AMD64、ARM64 两种平台。分别以 Git、Actions、注册表和 NAS 实例核实源码、构建、发布及部署状态。 [版本编号](../../../scripts/cat_two_docker_release.py) · [发布复盘](references/development-log.md)
- 冒烟脚本覆盖 Cat_two 新装和容器重建。升级验证用 `--previous-image` 指定实际旧版镜像并使用隔离数据卷。 [冒烟脚本](../../../scripts/release_smoke_test.sh)

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
