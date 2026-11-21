# Cat_two 功能与实现

这份说明对应 Cat_two 的 `main` 分支。日常使用可以先看功能表。开发时可以从表里的源码入口追到页面、API 和数据。检查 NAS 上的实际版本时，可以访问 `/api/instance/profile` 查看 `version` 和 `commit`，再与 GitHub 上的提交核对。

## 网页里的功能

| 功能 | 现在的用法和行为 | 代码入口 |
| --- | --- | --- |
| 登录与初始化 | 首次启动按页面引导建立管理员账户。登录后进入笔记工作区，普通账户和管理员都有自己的笔记。 | [路由与登录保护](../web/src/router/index.tsx)、[账户权限](../server/api/acl_config.go) |
| 日历和笔记列表 | 侧栏「日历」打开原时间线的列表。点日期筛选当天笔记，通过筛选标签上的「×」清除。选中日期后，编辑器会给新笔记带上这一天。打开旧的按日链接或恢复历史日期，列表与侧栏会定位到对应日期。 | [首页](../web/src/pages/Home.tsx)、[侧栏日历](../web/src/components/StatisticsView/)、[旧链接跳转](../web/src/router/LegacyCalendarRedirect.tsx) |
| 排序和来源 | 「全部笔记」菜单可按创建或更新时间升序、降序查看。侧栏的热度和日期筛选也使用所选时间类型。来源菜单可选「全部」「网页端」或「未记录」。 | [菜单](../web/src/components/MemoFeedMenu.tsx)、[排序](../web/src/hooks/useMemoSorting.ts)、[筛选](../web/src/hooks/useMemoFilters.ts) |
| 搜索、标签和视图 | 搜索支持关键词和 CEL 表达式。点击标签可筛选笔记。把常用条件保存为视图后，可在侧栏重复使用。标签与日期可以一起筛选，普通浏览条件也可以从 URL 恢复。 | [搜索](../web/src/components/AppSidebar/QuickFindDialog.tsx)、[筛选状态](../web/src/contexts/MemoFilterContext.tsx)、[视图管理](../web/src/pages/MemoViews.tsx)、[CEL 字段与语法](../filter/README.md) |
| 编辑和草稿 | 支持富文本编辑、Markdown 源码模式、清单、代码、链接及专注编辑。新笔记草稿按账户和写作位置保存在当前浏览器。已有笔记在原卡片或详情页编辑，保存后服务端更新。 | [编辑器说明](../web/src/components/MemoEditor/README.md)、[保存流程](../web/src/components/MemoEditor/edit/hooks/useMemoSave.ts) |
| 笔记内附件 | 从编辑器添加文件、图片、视频或音频，在笔记内预览和下载。部分图片支持涂画标注，媒体元数据可在偏好设置中开启。旧的独立附件库链接进入未找到页面。 | [附件编辑与显示](../web/src/components/MemoMetadata/Attachment/)、[上传 API](../server/api/attachment_upload.go)、[文件服务](../server/fileserver/) |
| 整理与详情 | 笔记可置顶、归档、恢复或删除。详情页提供目录、笔记引用和评论，可对笔记加表情反应，也可将笔记导出为图片。归档列表按当前账户查阅。 | [卡片菜单](../web/src/components/MemoActionMenu/)、[详情页](../web/src/pages/MemoDetail.tsx)、[归档页](../web/src/pages/Archived.tsx) |
| 空间 | 可以把笔记放在某个空间，切换空间查看相应笔记，或在设置管理空间和成员。空间控制组织范围和成员操作。笔记内容由服务端校验创建者身份。 | [空间上下文](../web/src/contexts/SpaceContext.tsx)、[空间设置](../web/src/components/Settings/SpacesSection.tsx)、[空间服务](../server/api/space_service.go) |
| 导出和导入 | 「全部笔记」可多选最多 100 条后导出 ZIP，也可在「设置 → 导出和导入」导出自己的全部笔记。导入先读取并预演归档，遇到同标识的笔记可保留现有、覆盖或创建副本。最后返回逐项处理报告。 | [导出和导入页面](../web/src/components/Settings/MemoExportSection.tsx)、[ZIP 客户端](../web/src/lib/memo-export.ts)、[归档格式](../core/memoexport/format.go) |
| 外观和设备 | 默认简体中文，可切换英语。主题可选跟随系统、浅色、深色和纸张。普通笔记正文为 14px，行高 25.2px，英文和数字使用内置 Barlow，中文按系统可用字体回退。网页及手机主屏幕使用小猫图标，默认显示名称为 Memos，浏览器标签标题可在站点设置中调整。 | [语言](../web/src/i18n.ts)、[主题](../web/src/themes/)、[笔记字体](../web/src/index.css)、[应用图标](../web/public/site.webmanifest) |
| 账户与实例设置 | 账户可管理资料、密码、个人访问令牌、编辑偏好、标签和 Webhook。管理员可配置站点、存储、邮件通知、单点登录、语音转写和实例配置。 | [设置项清单](../web/src/components/Settings/settingSections.ts)、[声明式配置](../docs/design/declarative-configuration.md) |

## 前端、服务端和数据库

```mermaid
flowchart LR
  A["浏览器页面与编辑器"] --> B["React Query 和 API 客户端"]
  B --> C["Go API 与访问校验"]
  C --> D["Store 和数据库驱动"]
  D --> E["SQLite"]
  C --> F["文件服务与附件存储"]
  C --> G["SSE 变更通知"]
  G --> B
```

- **前端**使用 React、TypeScript 和 Vite。[页面路由](../web/src/router/index.tsx)决定入口，[上下文](../web/src/contexts/)管理当前空间、筛选和显示设置，[请求 hooks](../web/src/hooks/)管理服务端数据及缓存。
- **服务端**由 [Go 命令入口](../cmd/memos/main.go)启动并执行数据库迁移，[HTTP 服务](../server/server.go)连接 API、文件服务、嵌入式网页、MCP 和 `/healthz`。[业务规则](../core/)和 [Store](../store/doc.go)分开维护。API 定义在 [Proto](../proto/api/)，REST 接口可查 [OpenAPI](../proto/gen/openapi.yaml)。
- **数据库**随普通和 NAS 配置使用 SQLite，默认生产数据库文件为 `memos_prod.db`。服务端也有 MySQL 和 PostgreSQL 驱动。SQLite 使用 WAL，数据库与配套文件应纳入文件级备份。[数据库配置](../internal/profile/profile.go) · [驱动选择](../store/db/db.go) · [SQLite 实现](../store/db/sqlite/sqlite.go)

### 一条笔记怎么保存

1. 首页根据当前账户、空间和日期准备编辑器。侧栏选择的日期缓存在 URL 的 `displayTime` 筛选中，前端以 `Asia/Shanghai` 计算当天的起止时间。
2. 编辑器先上传需保存的附件，再将 Markdown、附件引用和必要的时间信息发给 API。创建时服务端记录作者，处理笔记属性及附件绑定。更新时使用字段掩码提交改过的字段。
3. 服务端在数据库中写入笔记，刷新标签与内容属性。当前页面更新查询缓存，其他打开的页面通过 `/api/sse` 收到更新信号后按各自权限重新取数。

[日期工具](../web/src/lib/calendar-utils.ts) · [编辑器服务](../web/src/components/MemoEditor/edit/services/memoService.ts) · [笔记服务](../server/api/memo_service.go) · [缓存更新](../web/src/hooks/useMemoQueries.ts) · [实时刷新](../web/src/hooks/useLiveMemoRefresh.ts)

### 私有访问和输入来源

- 笔记详情、列表、统计和附件读取走后端的创建者检查。旧的公开或受保护笔记也使用这套规则，旧公开分享链接已停用。复制详情地址可以用于自己登录后打开。笔记内的空间分组和权限检查分别处理。[访问规则](../core/access/memo.go) · [API 访问范围](../server/api/memo_access.go) · [文件读取](../server/fileserver/fileserver.go)
- 网页保存会标记 `web`，较早或未标记的笔记显示「未记录」。接口预留了 Telegram、浏览器扩展和微信的来源值。接入其他输入渠道时，需要明确登录方式、写入账户和来源标记，再把此处的实际支持情况更新到功能表。[来源字段](../proto/api/memo_service.proto) · [来源校验](../server/api/memo_create_helpers.go)
- 地图和定位入口、独立日历页与附件库已移除。旧笔记的位置元数据仍可随 ZIP 来回保存，常用的日期工作流集中在日历笔记列表。[API 变更记录](../proto/api/CHANGELOG.md)

## 数据、备份和升级

| 对象 | 存放位置和用途 |
| --- | --- |
| 笔记、账户、空间与实例配置 | SQLite 实例使用 `/var/opt/memos` 下的数据库。普通 Compose 使用 `cat_two_data` 命名卷，实际卷名含 Compose 项目名前缀。重建容器时使用同一个项目名和数据卷。 |
| 附件 | 可以存入数据库、本地文件或 S3 兼容存储，具体由实例设置决定。配置本地文件时默认模板位于数据目录下的 `assets/`。使用相对模板新增的附件跟随数据目录。显式绝对路径及已有绝对路径记录按原位置读取。 |
| NAS 数据 | [NAS Compose](../compose.nas.yaml)默认把 `/volume1/docker/cat2/data` 挂载到 `/var/opt/memos`。已有实例继续挂载存放数据的原路径，应用在容器内使用配置好的 UID 和 GID 读写。 |
| 浏览器草稿与设备偏好 | 草稿、笔记列表显示选项及部分设备偏好保存在当前浏览器。账号语言、主题和 Markdown 模式等设置可随账户在设备间使用。保存后的笔记进入服务端。 |
| 笔记 ZIP | 适合阅读、迁移和恢复个人笔记。包括 `manifest.json`、每篇笔记的 Markdown 和 JSON，以及可归档的附件。导入后查看报告，核对日期、附件、归档状态和引用。 |

完整实例恢复使用文件级备份。备份时先停止服务或使用一致性快照，保存整个数据目录及部署配置。附件若配置在数据目录外或 S3，还需备份相应位置。升级时保持同一个数据挂载。需要在另一处测试恢复时，可以使用备份副本和独立的 Compose 项目。

[部署与升级命令](../README.md#部署服务端) · [NAS 配置](../README.md#在-nas-上部署) · [ZIP 导出](../server/api/user_service_memo_export.go) · [ZIP 导入](../server/api/user_service_memo_import.go)

用 ZIP 跨实例迁移时，请把已归档笔记的评论和大附件列入核对。已归档的父笔记可能阻止评论恢复，高压缩比的大附件可能触发 ZIP 读取限制。覆盖导入时，如果归档中没有笔记引用，已有引用会保留。[评论校验](../store/memo_comment.go) · [ZIP 读取限制](../core/memoexport/reader.go) · [引用恢复](../server/api/user_service_memo_import.go)

## 外部集成

| 入口 | 能力 |
| --- | --- |
| REST / Connect | Web 客户端使用 Connect，第三方可以通过 `/api/…` 的 REST 接口或 `/memos.api.…` 的 Connect 接口调用服务。个人访问令牌在设置中创建，持有该令牌的程序按对应账户权限访问。 |
| Webhook | 账户可配置对外发送的笔记创建、修改、删除等事件。Webhook 采用异步队列，可配置签名。 |
| MCP | 服务挂载 `/mcp`，外部 MCP 客户端可带着访问令牌操作同一个账户的笔记、附件和空间。工具调用仍走原有 API 权限。 |
| 语音转写 | 管理员为实例配置支持的 AI 服务后，编辑器可将音频转成文字。 |
| 手机访问 | 当前仓库交付的是 Web 服务和容器镜像。支持的浏览器可以添加到主屏幕。原生 Android 客户端可作为独立项目，调用同一个服务端。 |

[API 说明](../proto/api/README.md) · [Webhook](../server/api/memo_service_webhooks.go) · [MCP](../server/mcp/README.md) · [语音转写](../server/api/ai_service.go)

## 开发、验证和发布

日常开发用仓库根目录启动 Go 服务，在 `web/` 启动 Vite。Go、Node 和 pnpm 版本以 [go.mod](../go.mod) 和 [package.json](../web/package.json) 为准。验证时使用独立数据库，便于安全测试归档、删除和恢复。

```sh
# Terminal 1
go run -buildvcs=true ./cmd/memos --port 8081 --driver sqlite --data "$(mktemp -d)"

# Terminal 2
cd web
pnpm install --frozen-lockfile
pnpm dev
```

| 改动 | 核对方式 |
| --- | --- |
| 编辑、日历和界面 | 在 `web/` 执行 `pnpm lint`、`pnpm test`、`pnpm build`。在桌面和手机核对登录、写笔记、切换后返回历史日期、打开附件。 |
| API、数据和迁移 | 在仓库根目录执行 `go test ./...`，按改动重点运行 `server/api/test`、`server/fileserver`、`core` 或 `store` 套件。用两个账户验证各自笔记和附件的读取权限。 |
| 导入和导出 | 用带图片、标签、笔记引用和空间的测试笔记导出 ZIP，再在独立实例里试导入。确认首次导入、跳过、覆盖和创建副本的结果，检查重复导入后的图片显示。 |
| Compose 和镜像 | 用 `docker compose config --quiet` 和 `docker compose -f compose.nas.yaml config --quiet` 检查配置。使用镜像时验证 `/healthz`，再重建容器确认数据卷仍可读写。 |

`pnpm release` 将网页构建到 `server/frontend/dist`，Go 二进制将其嵌入。Docker 的构建和发布由 [Cat_two Container 工作流](../.github/workflows/cat-two-container.yml)完成。向 `main` 推送会触发验证，使用工作流的手动触发或 `[publish dockerhub]` 标记可发布 AMD64 和 ARM64 镜像。Docker Hub 编号从 `0.01` 起，每次发布递增 `0.01`。发布后核对编号、`latest` 和 `sha-<完整提交 SHA>` 是否指向同一镜像，再更新 NAS。[编号逻辑](../scripts/cat_two_docker_release.py)

改变用户可见行为时，同步更新功能表和对应的核对方式。
