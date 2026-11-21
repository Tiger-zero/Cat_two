---
name: cat-two-development
description: 在 Cat_two 仓库开发、排查缺陷、验证升级和交付修改。涉及网页体验、个人笔记、数据库迁移、NAS 部署、Docker 镜像或开发中断后恢复时使用。
---

# Cat_two 开发

## 每次修改都更新本文件

每次修改 Cat_two 的代码、配置、文档或 Skill，交付前都要在本文件的开发记录中补一条。发布镜像或收到 NAS 实测反馈后，补充对应记录。同一事项的多个提交可以合在一起写，保留各自有用的提交链接。

每条用上海时间记录日期、改了什么和为什么、验证结果与可复用的经验。能确认时附上提交、构建记录和镜像标签。分别记录源码是否推送、检查是否通过、镜像是否已发布以及用户是否已在 NAS 验证。未验证的事项注明范围和原因。保留以后开发能用到的结论和排查步骤。

## 产品和代码入口

Cat_two 从 Fumos 和 Memos 衍生而来，以 `Tiger-zero/Cat_two` 的 `main` 为日常开发分支。当前交付是 Docker 服务和网页，默认使用 SQLite，适合在 NAS 上管理个人笔记。产品现状和部署方式分别见 [功能与实现](../../../docs/cat-two-guide.md) 和 [README](../../../README.md)。

- 网页和主题在 `web/`，API 和文件服务在 `server/`，数据和迁移在 `store/`，接口定义在 `proto/`。按 [仓库开发规范](../../../AGENTS.md) 复用现有调用路径和测试。
- 默认简体中文，仅提供简体中文和英语。日历、笔记日期和日期筛选统一使用 `Asia/Shanghai`。调整局部颜色或字体时，先找实际渲染组件和主题变量，再核对相邻状态与主题。
- 个人笔记的权限由服务端检查。移除入口时一起核对路由、设置、API 和数据依赖。升级时保留旧有笔记、附件及关联数据，使原作者仍能访问自己的内容。

## 开发记录

日期按上海时区记录，提交可用于回看变更动机。

### 2026 年 11 月 20 日至 21 日的开发基线

| 日期 | 当时的工作 | 后续开发应核对 |
| --- | --- | --- |
| 11 月 20 日 | 从 Fumos 导入基线，建立 GitHub Actions 容器构建和 Docker Hub 发布。修复 NAS 数据目录的 UID/GID 配置，把健康检查固定在容器回环地址。 [NAS 权限提交](https://github.com/Tiger-zero/Cat_two/commit/ddba0c6e7) | 复用原数据挂载。出现启动失败时先区分数据目录权限、进程启动和 HTTP 探测。普通与 NAS 两份 Compose 都需要与实际镜像的环境变量匹配。 |
| 11 月 20 日至 21 日 | 改为中文和上海时间，调整界面、字形和猫图标。用「日历」承接原时间线及按日筛选，移除地图、独立附件库和独立日历页。 [日历与附件修正](https://github.com/Tiger-zero/Cat_two/commit/e822b27de) | 回放「点日期 → 新建笔记 → 清除筛选 → 返回该日期」，核对有笔记及无笔记日期、日期边界、旧链接和笔记内附件预览。 |
| 11 月 21 日 | 加入回收站、标签置顶和目录重命名、来源筛选及外部接入，修复历史升级测试夹具。 [功能提交](https://github.com/Tiger-zero/Cat_two/commit/ca1b0c88f) · [夹具修正](https://github.com/Tiger-zero/Cat_two/commit/c472d1bf4) | 核对移入和恢复后的状态及附件。标签重命名到已有同级标签时应合并两边笔记，并迁移子路径。迁移用例要按旧版本当时的表和列准备数据，迁移后再检查新结构。 |
| 11 月 21 日 | 首次创建管理员后关闭网页注册，停用空间等设置模块，加入可选的验证器两步验证和容器终端恢复。 [个人模式和两步验证](https://github.com/Tiger-zero/Cat_two/commit/51bfcff31) | 同时测试首次安装和旧数据升级。核对旧空间笔记的作者权限、启用后的动态码登录和防重放，以及终端重置后的旧会话失效和原密码登录。 |
| 11 月 21 日 | 建立 Docker Hub 编号规则，将 Memos 许可文本打包到镜像。 [编号规则提交](https://github.com/Tiger-zero/Cat_two/commit/b33aa59d5) · [镜像许可提交](https://github.com/Tiger-zero/Cat_two/commit/0292d1a55) | 发布时在注册表核对版本标签，并在镜像里检查许可文件。 |

### 11 月 21 日下午

- **验证和恢复说明**　两步验证开始读取数据库里的会话版本后，SSE 测试要先启动客户端并等到响应头，再断开连接。README 重写了部署、备份、升级，以及从运行中的 NAS 容器或 Compose 宿主机重置两步验证的步骤。重置后以原密码重新登录。 [SSE 测试修正](https://github.com/Tiger-zero/Cat_two/commit/636a14e45) · [README 重写](https://github.com/Tiger-zero/Cat_two/commit/4232a6a58)
- **两个仓库 Skill**　用 Cat_two 开发和 MCP 两个 Skill 取代继承的 Memos 发布 Skill。当时容器的 `/mcp` 使用个人访问令牌，后续改用专用连接令牌和审批。归档当前任务或按标签整理笔记时先预览，确认后再写入。容器发布继续走 Cat_two 的工作流。 [Skill 提交](https://github.com/Tiger-zero/Cat_two/commit/d70023930) · [MCP Skill](../cat-two-mcp/SKILL.md)
- **附件和 Markdown 工具栏**　回形针复用现有文件选择和保存流程。PDF、Word 和 Excel 在缺少标准 MIME 信息时按扩展名识别，下载保留原文件名。格式按钮复用编辑器命令，在富文本和 Markdown 源码模式都可用。测试时使用只含附件的笔记、真实文件和中文文件名，核对下载字节，还要验证倒序选区、组合选区及 360px 窄屏。 [提交](https://github.com/Tiger-zero/Cat_two/commit/feb68bc8b) · [编辑器说明](../../../web/src/components/MemoEditor/README.md)
- **站内文档下载修复**　`0.04` 的原生附件下载依赖刷新 Cookie，网页仍有有效访问令牌时可能出现「笔记能打开，附件无法下载」。在测试容器确认空格、括号及 `&` 的文件名可用，并复现 Cookie 缺失或失效时下载失败。修复后，站内文档用当前网页会话按附件 ID 取文件，保留原文件名，失败时在页面提示 HTTP 状态。后续排查可用状态码区分身份校验和文件存储问题。Chrome 的原生下载可能没有出现在网页的 Network 列表，当次 NAS 请求的错误码未能捕获。 [提交](https://github.com/Tiger-zero/Cat_two/commit/63e4e4c8f) · [下载实现](../../../web/src/lib/attachment-download.ts)
- **开发记录维护**　将每次修改后更新本文件的要求写入 Skill，并补齐下午的实现经验和已发布版本。验证 Skill 格式、仓库链接和文档差异。

当天的镜像发布都从 `main` 手动触发，核对了 AMD64、ARM64 以及编号、`latest` 和对应 SHA 标签。升级测试使用独立数据卷。

| Docker Hub 标签 | 源码提交 | 验证 |
| --- | --- | --- |
| `0.03` | `d70023930` | 从 `0.02` 升级后可读取旧笔记和保存新笔记，已发布镜像提供两步验证重置命令。 [构建记录](https://github.com/Tiger-zero/Cat_two/actions/runs/44883753022) |
| `0.04` | `feb68bc8b` | 从 `0.03` 升级通过，用已发布镜像实测图片、PDF、Word、Excel 以及格式按钮。 [构建记录](https://github.com/Tiger-zero/Cat_two/actions/runs/44886020145) |
| `0.05` | `63e4e4c8f` | 从 `0.04` 沿用测试数据卷升级。旧 Excel 附件在 Cookie 缺失或失效时仍能下载，文件名和字节一致。用户随后在自己的 NAS 确认原附件下载成功。 [构建记录](https://github.com/Tiger-zero/Cat_two/actions/runs/44887464527) |

`0.03` 的发布工作流通过。同提交的后端 CI 中，MySQL 的 `TestUserStore` 在删除账号时返回 `Error 1615`，提示预处理语句需要重新准备。本地 MySQL 套件及后续两版后端 CI 通过。再遇到类似问题时，先按驱动和用例单独复现，记录原始 SQL 错误。 [MySQL 测试日志](https://github.com/Tiger-zero/Cat_two/actions/runs/44883528899/job/136783787756)

### 11 月 21 日 17:29

- **Codex 笔记插件**　新增仓库插件目录和 `cat-two-notes@cat-two` v0.1.0。插件复用现有 `/mcp`，NAS 地址、令牌环境变量和工具范围由客户端配置。原仓库 MCP Skill 引用同一份查询、分组和确认后归档工作流。升级插件时用隔离的 Codex 配置检查实际安装、工作流命名空间和工具发现。 [插件安装与接入](../../../plugins/cat-two-notes/README.md) · [插件目录](../../plugins/marketplace.json)
- **验证范围**　插件清单和文档示例通过检查，Codex CLI 实测安装，App Server 在仓库外发现已启用的工作流。Docker Hub `0.05` 的独立实例验证 MCP 握手、鉴权、笔记读写与标签筛选。Codex 实际连接测试中默认发现四个只读工具，启用写入后可额外发现创建和更新工具。本次变更通过仓库交付，实测使用既有镜像和合成笔记，真实 NAS 接入待本地配置。

### 11 月 21 日 17:35

- **Codex 默认读写**　插件升至 `0.1.1`。接入示例提供四个读取工具和两个写入工具，写入仍需先预览并确认。补充 NAS 网页上的令牌入口、Windows 用户环境变量和已有 `[mcp_servers.cat_two]` 的更新方法。默认工具范围来自 Codex 本机配置，升级插件时已有连接也要更新工具列表。隔离环境已验证示例 TOML、插件清单和 CLI 从 `0.1.0` 升级至 `0.1.1`。 [NAS 接入说明](../../../plugins/cat-two-notes/README.md#连接-nas)
- **验证范围**　从现有 `0.05` 容器网页生成测试令牌，使用接入示例通过 Docker 私有网络调用六个 Codex 工具，创建、修改并筛选合成笔记。本次交付为插件与接入说明，验证环境为 Linux Codex CLI 和测试容器，真实 NAS 和 Windows 桌面接入待本地验证。

### 11 月 21 日 18:36

- **Cat_two MCP 连接审批**　网页签发专用令牌并仅显示一次。Codex 用 `/mcp` 地址和 `Authorization: Bearer …` 发起连接后，设置页显示待审批。可批准 24 小时或永久有效、拒绝或撤销。在 Personal 模式，普通 PAT 用于 REST，原 MCP 接入须更换令牌。批准后服务端提供六个笔记工具，并在每次请求检查有效期和撤销状态。内部分发按工具对应的 API 操作核对网关匹配结果。 [接入说明](../../../plugins/cat-two-notes/README.md#连接-nas)
- **升级和验证**　数据库结构升至 `26.11.4` 并持久化连接状态。SQLite、MySQL、MariaDB 和 PostgreSQL 通过连接和迁移测试。服务端覆盖并发处理首次请求及审批、审批过期和账号隔离。用独立实例、Chromium 和真实 Codex CLI 实测网页审批、用同一令牌重连、笔记读写及撤销。NAS 使用包含审批功能的镜像后可验证本地连接。

### 11 月 21 日 18:51

- **发布前修复存储 CI**　升级数据库结构后，「未来版本」夹具仍写死为现在的 `26.11.4`，四种数据库均报断言错误。测试从当前嵌入版本计算下一修订，随以后的迁移更新。MySQL 同次出现账号清理语句的 `1615`，回滚失败事务后仅对此错误重试整个删除事务，保留其他错误的处理。 [失败日志](https://github.com/Tiger-zero/Cat_two/actions/runs/44895305239/job/136819563071) · [暂停的发布](https://github.com/Tiger-zero/Cat_two/actions/runs/44895938286)
- **MySQL 发布复测**　全量测试并行迁移时，MySQL 删除账号主记录所用的预处理语句持续返回 `1615`，事务重试仍可耗尽。定位到最后一条带级联的删除后，只将这条固定 SQL 改为直接执行。唯一写入 SQL 的参数是已解码的整数账号 ID。完整 MySQL 套件以 8 个测试并发通过。 [第二次失败日志](https://github.com/Tiger-zero/Cat_two/actions/runs/44896373212/job/136822797176) · [暂停的发布](https://github.com/Tiger-zero/Cat_two/actions/runs/44896373037)

### 11 月 21 日 19:08

- **Docker Hub 0.06**　用提交 `53968a153da7eb193f691c35ccfd52f7f0d457ef` 发布 MCP 连接审批和存储修复。`0.06`、`latest` 与源码 SHA 标签均指向 `sha256:8d42b6cf8ee1cefd817597aba6d5787947560372769194e528111e748ccee552`，包含 AMD64 和 ARM64。 [发布记录](https://github.com/Tiger-zero/Cat_two/actions/runs/44897126449) · [镜像标签](https://hub.docker.com/r/tiger2896/cat_two/tags)
- **正式镜像升级验证**　独立测试卷从 `0.05` 升级到 `0.06`，SQLite 从 `26.11.3` 迁移至 `26.11.4`。旧私有笔记与 REST PAT 可用。使用新令牌实测等待审批、24 小时批准、六个工具调用、中文标签查询和撤销。重启后新笔记与撤销状态都保留。真实 NAS 部署待验证。
- **最终 CI**　发布提交的前端和后端检查均通过，存储 CI 覆盖 SQLite、MySQL、MariaDB 和 PostgreSQL。`compose.nas.yaml` 的实际部署使用 SQLite。 [后端检查](https://github.com/Tiger-zero/Cat_two/actions/runs/44897126401) · [前端检查](https://github.com/Tiger-zero/Cat_two/actions/runs/44897126388)

### 11 月 21 日 19:29

- **MCP 令牌直接生效**　创建时选择 24 小时（默认，从创建时算起）或永久有效，保存后即可首次连接。设置页移除待审批和批准操作。Codex 的 Bearer 环境变量留空，仅在 `Authorization` 标头粘贴完整的 `Bearer …`。服务端逐次检查有效期和撤销状态并记录首次使用，旧已批准令牌沿用原有效期，旧等待和待审批令牌需重新创建。沿用 `26.11.4` 数据库结构。 [接入步骤](../../../plugins/cat-two-notes/README.md#连接-nas)
- **连接验证**　用真实 Codex 客户端确认首次连接即可使用六个工具，撤销后同一会话的下一次调用失败。在 Chromium 验证两种有效期、复制完整请求头和首次调用。Go 回归验证过期、旧令牌状态和账号隔离。四种数据库的首次使用、迁移与并发状态测试、前端 1,344 项测试和构建均通过。
- **旧数据兼容**　使用已发布的 `0.06` 镜像创建笔记和旧令牌，再用新版构建启动同一独立测试卷。旧笔记和已批准令牌的原有效期保留，旧待审批令牌仍停用，新签发令牌直接连接。本次验证使用隔离实例，NAS 新流程随包含此改动的镜像上线。

## 开发和验证

1. 检查工作区和目标提交，保留已有修改。按用户要完成的操作，从网页或 API 入口追到权限和数据层。先用相关的小范围测试验证，再运行与改动面相符的检查。
2. 网页改动在 `web/` 选择 `pnpm lint`、`pnpm test` 和 `pnpm build`。UI 行为用独立测试实例在浏览器回放用户的实际操作。翻译同时核对两种语言。
3. API 或登录改动根据范围运行 `go test -race ./server/...` 及对应包测试。访问控制覆盖登录前、本人和其他账号。涉及实时刷新时，等 SSE 响应实际打开后再断开测试客户端。
4. 数据库改动同时验证全新安装和旧版本升级。同步 SQLite、MySQL、PostgreSQL 的迁移与 `LATEST.sql`，用 MySQL 驱动验证 MariaDB。运行 [存储 CI](../../../.github/workflows/backend-tests.yml) 的测试命令。接口定义变化按规范重新生成 Go、TypeScript 和 OpenAPI。
5. 部署改动分别运行 `docker compose config --quiet` 和 `docker compose -f compose.nas.yaml config --quiet`。用临时数据验证 `/healthz`、写入笔记和容器重建后的数据。用户可见行为变化时更新功能说明。阶段完成时记录已改内容、实际通过的检查和剩余事项，便于继续。

## 提交和镜像发布

- 报告时写明对应提交和验证结果。已推送 `main`、构建通过、已发布镜像和 NAS 已升级分别以 Git、Actions、注册表和实例信息为准。
- 用户要求发布镜像时，使用 [Cat_two Container](../../../.github/workflows/cat-two-container.yml) 对 `main` 运行 `dockerhub` 发布。Docker Hub 编号由 [编号脚本](../../../scripts/cat_two_docker_release.py) 按 `0.01` 递增。验证编号、`latest` 和 `sha-<完整提交 SHA>` 指向同一次发布，并核对 AMD64 与 ARM64。
- `scripts/release_version.sh development-version` 生成应用内部版本。在现有部署上核对 `/api/instance/profile` 返回的 `commit`，结合使用的镜像标签判断功能是否已上线。
