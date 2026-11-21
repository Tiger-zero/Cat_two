# Cat_two 开发记录

## 2026-11-21（上海时间）

本次复盘核对到 GitHub `main` 提交 `86dcec0ea`。Docker Hub 的 `latest` 指向 `0.06`，源码为 `53968a153`。
晚间的令牌直接生效、有效期管理、查询优化、格式工具栏和四个 MCP 读取工具已推送到 `main`。21:39 对应的后端检查与容器冒烟测试通过，镜像发布作业跳过。 [源码](https://github.com/Tiger-zero/Cat_two/commit/86dcec0ea) · [后端检查](https://github.com/Tiger-zero/Cat_two/actions/runs/44909964585) · [容器检查](https://github.com/Tiger-zero/Cat_two/actions/runs/44909964578) · [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two/tags)

### 界面、笔记和数据迁移

- 00:00–11:52 统一中文和英文笔记字体、浅色日历、居中布局及猫图标，让「日历」承接时间线和按日筛选。移除地图入口，服务端按作者限制笔记和附件访问，旧笔记的位置数据可随 ZIP 保存。
  回归时从侧栏选择历史日期，新建后清除筛选，再返回同一天核对时区、旧链接和附件。 [界面](https://github.com/Tiger-zero/Cat_two/commit/74816af09) · [权限和地图](https://github.com/Tiger-zero/Cat_two/commit/825c17ee6) · [字体](https://github.com/Tiger-zero/Cat_two/commit/05741a76c) · [日历与附件](https://github.com/Tiger-zero/Cat_two/commit/e822b27de)
- 11:52–14:24 修复 ZIP 重复导入时的行内附件引用和本地文件命名冲突。加入保留 30 天的回收站、标签置顶及目录重命名、来源筛选。
  旧版本迁移夹具按旧表结构准备数据，迁移后再查新列。复测回收站状态、旧附件、相同目标标签合并及其子标签路径。 [导入和存储修复](https://github.com/Tiger-zero/Cat_two/commit/e822b27de) · [回收站和标签](https://github.com/Tiger-zero/Cat_two/commit/ca1b0c88f) · [迁移夹具](https://github.com/Tiger-zero/Cat_two/commit/c472d1bf4)
- 16:33–16:56 在编辑器加上传回形针和初版格式按钮，对缺失或通用 MIME 的 PDF、Word 和 Excel 按扩展名识别。`0.04` 的站内文档下载依赖刷新 Cookie，导致笔记仍可打开时附件也可能失败。用隔离容器复现 Cookie 缺失和过期，改为用当前网页会话下载，保留文件名。
  复测时检查只含附件的笔记、中文及标点文件名、文件字节与 HTTP 状态。Chrome 原生下载当时可能不在网页 Network 列表。 [初版工具栏](https://github.com/Tiger-zero/Cat_two/commit/feb68bc8b) · [会话下载修复](https://github.com/Tiger-zero/Cat_two/commit/63e4e4c8f)
- 21:11–21:15 在底部增加斜体和「更多格式」，丰富正文、标题、任务、表格、链接等编辑能力。两种模式共用 URL 校验，源码表格和分隔线插入时保留原段落。菜单应用格式后先更新草稿再交还焦点，避免紧接着保存漏掉变更。修复含字面百分号的站内锚点，并核对 Markdown、附件和归档格式。 [工具栏](https://github.com/Tiger-zero/Cat_two/commit/d47739f2f) · [格式清单提交](https://github.com/Tiger-zero/Cat_two/commit/8b66bcc96)
- 这轮工具栏验证使用浅色桌面和深色 360px Chromium，覆盖键盘菜单、倒序和组合选区、链接、附件、保存后重开。1,371 项前端测试、类型检查和生产构建通过。隔离 Codex CLI 写入及更新后原样读回中文概要，Chromium 检查标题、任务、表格、代码、提示块、脚注和链接。要精确保留 Markdown 写法，可通过 MCP 或 API 提交正文，也可在源码模式纯文本粘贴。 [编辑与渲染](../../../../web/src/components/MemoEditor/README.md#formatting-and-source-mode)

### 个人账户与两步验证

- 15:23 首次注册创建管理员，随后关闭网页注册，停用空间等设置模块，同时保留旧账户对其已有笔记、附件和关联数据的权限。新增可选 TOTP，覆盖动态码登录、防重放和恢复后旧会话失效。21:39 写明 Cat_two 的单账户范围和独立演进原则。移植需求时从 Cat_two 的正常入口追到权限、数据和升级路径。 [个人模式和 TOTP](https://github.com/Tiger-zero/Cat_two/commit/51bfcff31) · [单账户范围](https://github.com/Tiger-zero/Cat_two/commit/86dcec0ea)
- 15:25–15:36 更新 SSE 测试和部署、备份及两步验证恢复说明。依赖数据库会话版本的 SSE 测试需启动服务并等到响应头再断开。丢失验证器时，可在运行中的容器执行 `/usr/local/memos/memos two-factor reset --username <用户名>`，再用原密码登录并重新绑定。由宿主机操作时沿用真实部署的 Compose 文件、项目与环境。 [SSE 测试](https://github.com/Tiger-zero/Cat_two/commit/636a14e45) · [恢复说明](https://github.com/Tiger-zero/Cat_two/commit/4232a6a58) · [当前命令](../../../../README.md#重置两步验证)

### Codex 插件与 MCP

- 16:09–17:37 建立 Cat_two 开发与 MCP Skill，随后加入仓库 marketplace 和 `cat-two-notes@cat-two` v0.1.0。v0.1.1 默认配置四个读取工具和两个写入工具，归档前预览并确认。隔离 Codex CLI 实测安装和升级，App Server 在仓库外发现工作流，正式 `0.05` 的独立容器验证当时的 PAT 接入、读写和标签筛选。 [仓库 Skill](https://github.com/Tiger-zero/Cat_two/commit/d70023930) · [插件](https://github.com/Tiger-zero/Cat_two/commit/ac67e8f88) · [默认读写](https://github.com/Tiger-zero/Cat_two/commit/e5d52bdec)
- 18:38 加入专用 MCP 令牌与审批，数据库升级到 `26.11.4`。普通 PAT 继续用于 REST。专用令牌首次请求进入待审批，批准后可使用六个笔记工具。内部请求按实际匹配的网关操作绑定账号。SQLite、MySQL、MariaDB 和 PostgreSQL 的连接与迁移测试，以及 Chromium 和真实 Codex CLI 的审批、同一令牌重连和撤销验证通过。此流程随 `0.06` 发布。 [审批实现](https://github.com/Tiger-zero/Cat_two/commit/92db457d6) · [MCP 请求流程](../../../../server/mcp/README.md#request-flow)
- 19:31–20:21 后续源码在签发时直接激活令牌，最初为 24 小时或永久，随后定为 7 天（默认）、14 天、1 个日历年或永久。从签发起算，旧有效令牌沿用原到期时间。设置显示首次记录的客户端 IP 及北京时间到期时间。通过 `DELETE /api/mcpConnections/{id}` 删除本人令牌和记录，同一客户端下一次调用即被拒绝，旧撤销接口保留。Codex 使用完整的 `Authorization: Bearer cat_two_mcp_…`，Bearer 环境变量留空。REST PAT 以 `memos_pat_` 开头。 [签发即生效](https://github.com/Tiger-zero/Cat_two/commit/4230681f2) · [令牌界面](https://github.com/Tiger-zero/Cat_two/commit/4d6e591f6) · [有效期和删除](https://github.com/Tiger-zero/Cat_two/commit/fe64e51c7) · [连接步骤](../../../../plugins/cat-two-notes/README.md#连接-nas)
- 用正式 `0.06` 在独立测试卷创建笔记和令牌，再用新版构建启动同一卷。旧笔记和既有批准令牌的到期时间保留，旧待审批令牌仍停用。
  真实 Codex CLI 验证直接连接、笔记读写和删除后失败，Chromium 验证复制完整请求头、有效期和删除。隔离环境验证覆盖 SQLite 全量存储及 MySQL、MariaDB、PostgreSQL 相关存储用例。授权复测用已连接客户端发起实际调用，工具列表可以被客户端缓存。 [连接详情](../../../../server/mcp/README.md#managing-cat_two-connections)
- 20:31 合并按令牌哈希查询连接和账号状态，网关收到已校验的账号 ID。SQLite 查询计划命中哈希唯一索引和账户主键。四种数据库的 MCP 测试覆盖连接读取，用仍缓存为正常的归档用户验证下一次调用拒绝。持续检查令牌、到期和归档状态。 [查询优化](https://github.com/Tiger-zero/Cat_two/commit/f38d9b73d)
- 21:32 参考 flomo 的笔记查询能力，增加 `memo_search`、`memo_batch_get`、`tag_tree` 和 `tag_search` 四个个人模式只读工具。搜索复用 CEL 和原列表，日期按上海时区的创建日计算并包含首尾。批量读取最多 50 个笔记名，保留请求顺序和原 Markdown。标签按路径分页，祖先标签对同一笔记只计一次。SQLite 集成测试覆盖上海午夜边界、分页、归档、回收站和账号隔离。真实 Codex CLI 验证工具发现、中文标签、正文读取、删除令牌后的拒绝。 [四个工具](https://github.com/Tiger-zero/Cat_two/commit/886426924) · [字段与范围](../../../../server/mcp/README.md#tool-surface) · [参考](https://help.flomoapp.com/advance/mcp/index.html#开放了哪些能力)

### 发布、升级和 CI 诊断

11 月 20 日从 Fumos 接续源码并建立 Docker 构建，加入 NAS 权限配置和容器回环健康检查。更新实例时沿用原数据目录、Compose 项目名和账号 UID/GID。启动问题按数据目录权限、服务进程、探测请求依次核对。 [NAS 权限](https://github.com/Tiger-zero/Cat_two/commit/ddba0c6e7) · [NAS 部署](../../../../README.md#在-nas-上部署)

| Docker Hub | 提交 | 发布与验证 |
| --- | --- | --- |
| `0.01` | `b33aa59d5` | 建立从 `0.01` 起按 `0.01` 递增的发布编号。 [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/44865933558) |
| `0.02` | `0292d1a55` | 镜像包含 Memos 的许可文本。 [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/44876618627) |
| `0.03` | `d70023930` | 独立卷自 `0.02` 升级后可读写笔记，镜像包含两步验证重置命令。 [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/44883753022) |
| `0.04` | `feb68bc8b` | 独立卷自 `0.03` 升级通过，验证图片、PDF、Word、Excel 及格式按钮。 [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/44886020145) |
| `0.05` | `63e4e4c8f` | 独立卷自 `0.04` 升级，确认刷新 Cookie 缺失或失效时站内 Excel 可下载。用户在自己的 NAS 随后确认原 Excel 附件下载成功。 [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/44887464527) |
| `0.06` | `53968a153` | 独立卷自 `0.05` 升级，SQLite `26.11.3` 升到 `26.11.4`，旧私有笔记和 REST PAT 可用，验证审批、六个工具、中文标签、撤销及重启后的持久化。前端及四驱动后端 CI 通过。 [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/44897126449) · [后端](https://github.com/Tiger-zero/Cat_two/actions/runs/44897126401) · [前端](https://github.com/Tiger-zero/Cat_two/actions/runs/44897126388) |

`0.06`、`latest` 和 `sha-53968a153da7eb193f691c35ccfd52f7f0d457ef` 共用 `sha256:8d42b6cf8ee1cefd817597aba6d5787947560372769194e528111e748ccee552`，包含 AMD64 与 ARM64。已发布镜像的升级测试使用独立卷。私有 NAS 的验证记录是 `0.05` 的原 Excel 下载，MCP 浏览器与 Codex 验证使用隔离实例。 [镜像列表](https://hub.docker.com/r/tiger2896/cat_two/tags)

- `0.03` 容器发布通过，同提交的 Store CI 中 MySQL 的 `TestUserStore` 删除账号时返回 `Error 1615 (HY000): Prepared statement needs to be re-prepared`。本地 MySQL 与 `0.04`、`0.05` 的后端 CI 后续通过。 [日志](https://github.com/Tiger-zero/Cat_two/actions/runs/44883528899/job/136783787756)
- `0.06` 发布前两次候选取消。第一次四驱动的 `TestMigrationRejectsUnsupportedSchema/calver-newer` 仍把 `26.11.4` 当作未来版本，MySQL 另在 `TestMCPConnectionStore` 的账号删除遇到 `1615`。
  将夹具改为按嵌入的当前版本计算下一修订，并限制事务重试只处理该错误。第二次并行迁移仍使 MySQL 最后一条带级联的账号删除语句重试耗尽，故将这条固定 SQL 改为直接执行，参数使用已解码的整数账号 ID。MySQL 全量以 8 个并发通过。
  [修复夹具](https://github.com/Tiger-zero/Cat_two/commit/8d8868c67) · [删除修复](https://github.com/Tiger-zero/Cat_two/commit/53968a153) · [首次日志](https://github.com/Tiger-zero/Cat_two/actions/runs/44895305239/job/136819563071) · [复测日志](https://github.com/Tiger-zero/Cat_two/actions/runs/44896373212/job/136822797176) · [候选一](https://github.com/Tiger-zero/Cat_two/actions/runs/44895938286) · [候选二](https://github.com/Tiger-zero/Cat_two/actions/runs/44896373037)
- 发版核验编号、`latest`、对应 SHA 标签及平台清单，从独立卷验证旧版本升级和数据回读。以 Git、Actions、注册表及实际实例分别核对源码、检查、镜像和部署。NAS 升级后可用 `/api/instance/profile` 的 `commit` 对照镜像。 [发布工作流](../../../../.github/workflows/cat-two-container.yml) · [部署说明](../../../../README.md#备份和升级)

### P1 与 P2 修复

- **编辑与搜索。** 保留同一上海日期下的手动时间，恢复当前浏览器中的已有笔记草稿和待上传附件。保存前检查笔记版本，避免慢上传期间的更新被覆盖。隔离 SQLite 和 Chromium 核对刷新、暂存文件原始字节、取消编辑、多个标签页和服务器版本冲突。归档范围搜索覆盖日期、标签、同一查询多页、浏览器返回及 360px 深色界面。 [编辑器](../../../../web/src/components/MemoEditor/README.md) · [搜索回归](../../../../web/tests/quick-find-navigation.test.tsx)
- **归档与回收站。** ZIP 恢复归档笔记下同一作者的评论，测试覆盖导入冲突策略和高压缩比附件字节。清理任务随彻底删除一并入库。测试模拟清理失败并重开数据库，也验证了旧的绝对路径、共享文件和 S3 同名对象。 [导入回归](../../../../server/api/test/memo_export_test.go) · [清理回归](../../../../server/api/memo_trash_handler_test.go)
- **存储与查询。** 增量迁移将数据库架构升级到 `26.11.6`。SQLite 新索引覆盖默认置顶排序，MCP 标签改为单次流式读取。对 SQLite 运行存储全套及服务端和核心竞态测试，验证 MySQL、MariaDB 和 PostgreSQL 的相关迁移及存储用例。 [索引回归](../../../../store/test/migrator_memo_index_test.go) · [查询基准与约束](optimization-plan.md#p2--查找与查询)
- **整体验证。** Go 静态检查和现代化检查通过，前端类型检查、1,378 项全量测试及生产构建通过。本地标准 Docker 构建遇到 Alpine 软件源证书错误，改用 `0.06` 的运行层封装新编译程序。隔离容器通过新装、重建和从 `0.06` 升级后的笔记读写验证。 [冒烟脚本](../../../../scripts/release_smoke_test.sh)
