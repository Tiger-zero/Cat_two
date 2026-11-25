# Cat_two 开发记录

## 2026-11-25（上海时间）

### 镜像 0.18

22:35 核对 Docker Hub `0.18`、`latest` 和 `sha-37f5677f4ff90475fb60b6f532c8514b97ee02c9` 均指向 `sha256:0b5d913ceb78bed8645e6741dab320db73491aba6848bf9d0b977381f01383bb`。Linux AMD64 与 ARM64 镜像均对应提交 `37f5677f4`，两个平台的新装及同卷重建验证通过。 [发布工作流](https://github.com/Tiger-zero/Cat_two/actions/runs/45634186756) · [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two/tags?name=0.18)

本版将浅色首页及回收站画布设为纯白，首页已保存笔记使用 1px 纯黑边框。深色桌面首页和回收站侧栏同步浅色版的间距与无边框布局，日历以灰色与绿色方块区分空日期和有笔记的日期。 [桌面配色](../../cat-two-ui-development/references/desktop-visual-design.md)

### 镜像 0.17

00:25 发布 Docker Hub `0.17`。已从镜像仓库核对编号、`latest` 和 `sha-51e80b2c5b88f47aea24f706560b4bd5c26f4980` 均指向 `sha256:c423335e624239a1a12172fe2a5978abf205e1afde851b91ca24e11f014c417d`。Linux AMD64 和 ARM64 的 OCI 标签均对应提交 `51e80b2c5`。 [发布工作流](https://github.com/Tiger-zero/Cat_two/actions/runs/45435539026) · [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two/tags?name=0.17)

本版使用单管理员 SQLite 结构和 MCP 专用令牌。新装使用空数据卷，当前版本可用原卷重建。用实际 `0.16` 镜像建库后验证启动保护，误挂旧卷时文件校验值、权限及所有者均保持一致。Go 静态检查、服务端竞态测试、1,410 项前端测试和 Chromium 操作验证通过。两个原生平台分别验证正式镜像的新装、刷新会话、读写笔记、附件字节、回收站恢复和同卷重建。 [部署说明](../../cat-two-product-development/references/deployment.md#备份和升级) · [接口说明](../../cat-two-product-development/references/external-api.md)

发布流程在每个任务内读取公开仓库名，避免 GitHub 对跨任务输出的脱敏造成镜像地址缺失。构建缓存服务暂时无法写入时，发布流程可继续验证实际镜像。 [构建工作流](../../../../.github/workflows/cat-two-container.yml)

## 2026-11-23（上海时间）

### 镜像 0.16

23:17 发布 Docker Hub `0.16`。`0.16`、`latest` 和 `sha-0c7f0ef9638c58fd0e0757a0e7ce3cd24e8e11b5` 指向同一镜像索引 `sha256:0850a1b4035ca76d5cdecc3824904d0f007fa9700be3542439346bb5c8b2ecb8`。Linux AMD64 和 ARM64 均对应提交 `0c7f0ef96`。本版更新浅色主页画布、桌面侧栏和日历配色，并为经 MCP 或个人访问令牌新建的笔记添加 `#MCP连接`。 [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/45202778224) · [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two/tags?name=0.16)

1444 项前端测试和后端 CI 通过。正式 AMD64 镜像通过隔离 SQLite 卷的新装、重建和从 `0.15` 升级验证。Chromium 核对浅色预览配色和按日筛选，实际 MCP 和 PAT 创建的笔记可从侧栏按 `#MCP连接` 找回。 [前端检查](https://github.com/Tiger-zero/Cat_two/actions/runs/45202778133) · [后端检查](https://github.com/Tiger-zero/Cat_two/actions/runs/45202778322)

## 2026-11-22（上海时间）

00:28 发布 Docker Hub `0.07`。`0.07`、`latest` 和 `sha-da3bb55d5348c74d46db5514cceab6c8b4cec0ea` 共用 `sha256:6cc27f7ccd216a4e455a63cac1269e7a701a64792a2f05554244fd6451f57cb3`，AMD64 和 ARM64 均对应提交 `da3bb55d5`。发布工作流的容器检查及本地 AMD64 使用正式镜像的新装、重建和从 `0.06` 升级检查通过。 [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/44924203110) · [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two/tags?name=0.07)

01:21 核对 Docker Hub `0.08`。`0.08`、`latest` 和 `sha-f3f74e235c0b6c8087a59281b58d4a84d22aca03` 共用 `sha256:d462b78e8bdf44fba17e1cef81ccceb7dc3c52839e774d73620a43a8af4de8d7`，AMD64 和 ARM64 均对应提交 `f3f74e235`。正式镜像在独立 SQLite 卷上从 `0.07` 升级，原账户登录、旧笔记读取和新笔记写入通过，旧视图记录原样保留，旧接口返回 `404`。本地 AMD64 Chromium 检查设置弹窗、回收站侧栏、选中颜色及旧 `/views` 链接。 [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/44929146492) · [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two/tags?name=0.08)

### 对外接口与公网加固

- 整理 REST、MCP、文件下载、回收站、ZIP、SSE 和消息回调的实际入口。记录 PAT、MCP 连接令牌和网页登录的权限边界、分页规则、Markdown 写回和重试。 [外部接口](../../cat-two-product-development/references/external-api.md)
- 会话刷新令牌按单实例 SQLite 部署原子轮换，凭据变更以数据库为准。HTTPS 公网配置强制使用安全 Cookie，安全上下文的多标签页串行刷新。附件读权限前置，Connect 大上传先验证身份，个人 MCP 上限调为 16 MiB。企业微信握手只返回经过验证的挑战。代理信任默认关闭，公网文档给出 HTTPS 回源和本机绑定。 [公网部署](../../cat-two-product-development/references/deployment.md#在公网部署)
- 用 `govulncheck` v1.10.0 扫描 Go 的 Linux AMD64 生产构建及原 `0.08` 发布产物，漏洞库快照为 2026-11-11。报告的 `GO-2026-5932` 涉及 `golang.org/x/crypto/openpgp`。用生产构建标签核对当前和 `0.08` 源码的依赖闭包，两者均未引入该包。发布二进制剥离符号，二进制扫描精度限于模块。 [Go 公告](https://pkg.go.dev/vuln/GO-2026-5932)
- 更新前端依赖锁。`pnpm audit --prod` 从 21 条降至 0 条，完整 `pnpm audit` 从 57 条降至 0 条。冻结安装、类型和样式检查、前端构建通过。补齐测试临时目录遗漏的共享夹具后，涉及的测试复测通过，两轮覆盖 1,346 项。Chromium 本机安全上下文中的两个标签页共享会话时只发起一次刷新。 [前端依赖](../../../../web/package.json) · [刷新测试](../../../../web/tests/session-refresh.test.ts)
- Docker 运行层更新至受支持的 Alpine 3.24，构建时升级系统包。AMD64 候选的 zlib 和 OpenSSL 已达到 Alpine 安全库列出的修复版本，ARM64 核对官方软件包索引。 [Dockerfile](../../../../scripts/Dockerfile) · [Alpine 安全数据库](https://secdb.alpinelinux.org/)
- 后端、核心及 SQLite 存储的竞态测试通过。两份 Compose 验证了默认端口映射及本机绑定、HTTPS 基址、代理 IP 配置。用本轮源码、发布模式网页资源和升级后的运行层组装本地 AMD64 候选，在独立卷验证新装、重建及从 `0.08` 升级后的笔记读写。公网入口仍需按实际域名、防火墙和代理验收。 [冒烟测试](../../../../scripts/release_smoke_test.sh) · [部署说明](../../cat-two-product-development/references/deployment.md#在公网部署)

### 登录页检查与 IP 黑名单

- 用独立实例和虚构凭据核对公开页面、实例资料、用户接口和错误响应。未观察到管理员用户名、邮箱、密码、密码哈希或签名密钥对匿名调用者可见。账号不存在与密码错误返回一致，并对缺少密码哈希的账号执行同成本的 bcrypt 校验。登录及续期拒绝不可信浏览器来源，REST 用标准 HttpOnly Cookie 返回刷新令牌。旧 `Grpc-Metadata-Cookie` 续期请求仍可在未提供标准 Cookie 时使用。 [隐私与 Cookie 回归](../../../../server/test/login_privacy_test.go)
- SQLite 架构升至 `26.11.7`。IPv4 和 IPv6 地址统一规范化，连续 6 次密码失败后持久化封禁。完成登录清除未达阈值的计数。验收覆盖重启、并发计数、IPv6、分页、变造代理头、PAT 和非管理员拦截。真实 Chromium 验证先保留管理员会话，再由另一会话触发封禁、从安全设置解除及重新登录。 [黑名单回归](../../../../server/test/login_blocklist_test.go) · [部署主机恢复](../../cat-two-product-development/references/deployment.md#登录防护和-ip-解封)

### 镜像 0.09

10:49 核对 Docker Hub `0.09`、`latest` 和 `sha-1071616d72a604b35c95077dd3785f50d02fba6a`。三个标签共用镜像索引 `sha256:52f0239e11e64e13a310c653d6362f028714cd97641579daf3e8bfdd1fd12846`，Linux AMD64 和 ARM64 的镜像配置均对应提交 `1071616d`。发布工作流的构建和容器冒烟通过。本地用正式 AMD64 镜像和独立 SQLite 卷验证新装、重建以及从 `0.08` 升级后的笔记读写。 [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/44977510259) · [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two/tags?name=0.09)

### 镜像 0.10

17:28 发布 Docker Hub `0.10`。`0.10`、`latest` 和 `sha-0b9398d506e9b12d0895221fe7f203d7c57ab148` 共用镜像索引 `sha256:0edd2cbbe7fa6ed9da0211e528c49a1ddfb34aef87dc8f27da03bd16fe508e0b`。Linux AMD64 与 ARM64 均对应提交 `0b9398d50`。本版包含本地笔记和附件迁移、会话及 MCP 加固、可重试的笔记写入、通知与 Telegram 代理设置，以及开发 Skill 拆分。 [源码](https://github.com/Tiger-zero/Cat_two/commit/0b9398d506e9b12d0895221fe7f203d7c57ab148) · [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/45007206189) · [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two/tags?name=0.10)

用正式 AMD64 镜像和独立 SQLite 卷验证新装、容器重建和从 `0.09` 升级。Chromium 核对实例提交、通知配置、代理认证及配置持久化，并用签名加密的企业微信模拟消息验证笔记写入和 `Yes!` 回复。 [升级与备份](../../cat-two-product-development/references/deployment.md#备份和升级)

### 镜像 0.11

17:56 发布 Docker Hub `0.11`。`0.11`、`latest` 和 `sha-6b4bfd7631d7041823c594f7eb9e5f70124aa014` 共用镜像索引 `sha256:44aa6dee8549aff23246f1e3457413c48261d229436724d6c8b6675b04ab05a8`。Linux AMD64 与 ARM64 均对应提交 `6b4bfd763`。本版支持在企业微信通知设置中保存和复制带端口的公网回调地址。 [源码](https://github.com/Tiger-zero/Cat_two/commit/6b4bfd7631d7041823c594f7eb9e5f70124aa014) · [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/45009583745) · [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two/tags?name=0.11)

正式 AMD64 镜像通过独立 SQLite 卷的新装、重建和从 `0.10` 升级验证。Chromium 核对镜像提交、局域网 IP 提示、带端口的 HTTPS 地址保存与重启保留，并用模拟企业微信请求验证签名 GET 挑战和 POST 收件。 [企业微信配置](../../cat-two-product-development/references/deployment.md#企业微信自建应用)

### 镜像 0.12

18:44 核对 Docker Hub `0.12`、`latest` 和 `sha-7e7a961553981644edaa3ccb68c04b73346696dd`。三个标签使用同一镜像索引 `sha256:9825a6f0f6259bd954388b3c8dde403e6ac4058179b7d6bec230f5af72a4cdfb`，Linux AMD64 和 ARM64 的镜像配置均对应提交 `7e7a96155`。桌面主题覆盖浅色、深色、纸张和跟随系统，侧栏默认宽 320px，同版包含多级标签管理及无标签筛选。 [设计方案](../../cat-two-ui-development/references/desktop-visual-design.md) · [源码](https://github.com/Tiger-zero/Cat_two/commit/7e7a961553981644edaa3ccb68c04b73346696dd) · [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/45013388369) · [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two/tags?name=0.12)

1431 项前端测试与 SQLite 后端 CI 通过。正式 AMD64 镜像通过独立 SQLite 卷新装、重建和从 `0.11` 升级。Chromium 在 1024px 和 1440px 桌面窗口核对主题、侧栏宽度及独立滚动、固定编辑器、双列日历弹层、搜索和设置交互，并保存笔记后读回。 [前端检查](https://github.com/Tiger-zero/Cat_two/actions/runs/45013388066) · [后端检查](https://github.com/Tiger-zero/Cat_two/actions/runs/45013230016)

### 镜像 0.14

21:30 发布 Docker Hub `0.14`。编号、`latest` 和 `sha-65c54c8b75ab11b94f01d89af0b338cf6eb04e3a` 指向同一索引 `sha256:cdf7c36c53f5c96848ca65818a0170b131e73681fda99bd701ecbe11822f6322`，AMD64 与 ARM64 均对应提交 `65c54c8b7`。新版包含 `#121212` 深色面板和 Node.js 24 CI 工具链更新。 [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/45027528075) · [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two/tags?name=0.14)

正式 AMD64 镜像通过独立 SQLite 卷新装、重建及从 `0.13` 升级。Chromium 确认深色侧栏、编辑器、笔记、搜索按钮和设置面板的底色，并对照浅色和纸张主题。前端和 Proto CI 均通过。 [前端检查](https://github.com/Tiger-zero/Cat_two/actions/runs/45027528175) · [Proto 检查](https://github.com/Tiger-zero/Cat_two/actions/runs/45027528087)

### 镜像 0.15

22:07 发布 Docker Hub `0.15`。`0.15`、`latest` 和 `sha-b7ab7af94602bf36155db85cdd99c87ee02d748f` 指向同一镜像索引 `sha256:a33c472a9dacc10dd7d9ca7cd12f842c1bfd67f4028b17ddcc8f68a00fa2d22b`。Linux AMD64 和 ARM64 均对应提交 `b7ab7af94`。这版将浅色桌面首页的侧栏、账号面板、日历、输入框和笔记卡片设为纯白，设置导航复用标签的选中样式。 [发布](https://github.com/Tiger-zero/Cat_two/actions/runs/45030702784) · [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two/tags?name=0.15)

1444 项前端测试通过。正式 AMD64 镜像在隔离 SQLite 卷上通过新装、重建及从 `0.14` 升级。Chromium 截图与确认的浅色首页预览逐像素一致，并验证了三种主题下设置选择的颜色及鼠标、键盘交互。 [前端检查](https://github.com/Tiger-zero/Cat_two/actions/runs/45030702728)

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
- 15:25–15:36 更新 SSE 测试和部署、备份及两步验证恢复说明。依赖数据库会话版本的 SSE 测试需启动服务并等到响应头再断开。丢失验证器时，可在运行中的容器执行 `/usr/local/memos/memos two-factor reset --username <用户名>`，再用原密码登录并重新绑定。由宿主机操作时沿用真实部署的 Compose 文件、项目与环境。 [SSE 测试](https://github.com/Tiger-zero/Cat_two/commit/636a14e45) · [恢复说明](https://github.com/Tiger-zero/Cat_two/commit/4232a6a58) · [当前命令](../../cat-two-product-development/references/deployment.md#重置两步验证)

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
- 发版核验编号、`latest`、对应 SHA 标签及平台清单，从独立卷验证旧版本升级和数据回读。以 Git、Actions、注册表及实际实例分别核对源码、检查、镜像和部署。NAS 升级后可用 `/api/instance/profile` 的 `commit` 对照镜像。 [发布工作流](../../../../.github/workflows/cat-two-container.yml) · [部署说明](../../cat-two-product-development/references/deployment.md#备份和升级)

### P1 与 P2 修复

- **编辑与搜索。** 保留同一上海日期下的手动时间，恢复当前浏览器中的已有笔记草稿和待上传附件。保存前检查笔记版本，避免慢上传期间的更新被覆盖。隔离 SQLite 和 Chromium 核对刷新、暂存文件原始字节、取消编辑、多个标签页和服务器版本冲突。归档范围搜索覆盖日期、标签、同一查询多页、浏览器返回及 360px 深色界面。 [编辑器](../../../../web/src/components/MemoEditor/README.md) · [搜索回归](../../../../web/tests/quick-find-navigation.test.tsx)
- **归档与回收站。** ZIP 恢复归档笔记下同一作者的评论，测试覆盖导入冲突策略和高压缩比附件字节。清理任务随彻底删除一并入库。测试模拟清理失败并重开数据库，也验证了旧的绝对路径、共享文件和 S3 同名对象。 [导入回归](../../../../server/api/test/memo_export_test.go) · [清理回归](../../../../server/api/memo_trash_handler_test.go)
- **存储与查询。** 增量迁移将数据库架构升级到 `26.11.6`。SQLite 新索引覆盖默认置顶排序，MCP 标签改为单次流式读取。对 SQLite 运行存储全套及服务端和核心竞态测试，验证 MySQL、MariaDB 和 PostgreSQL 的相关迁移及存储用例。 [索引回归](../../../../store/test/migrator_memo_index_test.go) · [查询基准与约束](optimization-plan.md#p2--查找与查询)
- **整体验证。** Go 静态检查和现代化检查通过，前端类型检查、1,378 项全量测试及生产构建通过。本地标准 Docker 构建遇到 Alpine 软件源证书错误，改用 `0.06` 的运行层封装新编译程序。隔离容器通过新装、重建和从 `0.06` 升级后的笔记读写验证。 [冒烟脚本](../../../../scripts/release_smoke_test.sh)

## 历次开发简记

- **2026-11-21**　汇总单账户收敛、笔记编辑和附件、MCP 接入、存储验证及镜像进展。 [当日复盘](development-log.md)
- **2026-11-21 22:00**　重写开发 Skill，精简功能说明并迁移历史设计。文档链接、ZIP Schema 字节比对和归档、配置、限流测试通过。 [优化方案](optimization-plan.md)
- **2026-11-21 22:20**　完成笔记时间、草稿、归档导入和回收站清理修复。搜索支持归档范围，SQLite 默认列表和 MCP 标签读取已优化。 [P1、P2 修复与验证](development-log.md#p1-与-p2-修复)
- **2026-11-21 22:31**　将本地默认存储测试、后端 CI 和升级验证统一到 SQLite。默认存储全量、单项迁移筛选和工作流语法检查通过。 [存储测试命令](../../../../store/test/README.md)
- **2026-11-21 22:46**　按改动路径运行日常 CI，合并前端生产构建，移除 Codecov 和上游发布工作流。Stale 和旧版 Memos 升级改为手动运行。SQLite 全套、旧版升级、启动脚本和容器触发条件验证通过。 [工作流](../../../../.github/workflows/) · [容器 CI](https://github.com/Tiger-zero/Cat_two/actions/runs/44915807863)
- **2026-11-21 23:00**　关闭 Stale 及旧版 Memos 升级工作流，清理上游镜像测试和本地冒烟的自动旧镜像选择。SQLite 存储全套和 Cat_two 新装、重建及从 `0.06` 升级验证通过。 [存储测试](../../../../store/test/README.md) · [冒烟脚本](../../../../scripts/release_smoke_test.sh)
- **2026-11-21 23:15**　首页侧栏复用日历查询，展示未归档笔记、不同标签路径和上海时区的记录天数。调整树形及平铺标签颜色、置顶区分隔和子标签引导线，深色侧栏使用黑底白字。浏览器核对桌面和窄屏、跨午夜计数、保存后刷新和标签筛选与置顶。 [功能说明](../../cat-two-product-development/references/product-guide.md#网页里的功能)
- **2026-11-21 23:19**　将后续界面设计、实现和视觉验收范围明确为桌面 Web，核对开发规范和产品定位表述一致。 [功能说明](../../cat-two-product-development/references/product-guide.md)
- **2026-11-21 23:30**　调整桌面笔记页间距，默认侧栏宽 280px，主要区块间距 24px，视图和标签行高 32px，单列编辑器和笔记卡片间距 16px。核对浅色、深色、侧栏宽度保存、标签筛选和多列布局。前端静态检查、1,379 项测试和生产构建通过。 [功能说明](../../cat-two-product-development/references/product-guide.md#网页里的功能)
- **2026-11-21 23:50**　桌面首页标题和新笔记输入框随列表滚动吸顶。单列和多列共用同一编辑器，书写时展开并用 `RGB(48, 207, 121)` 画边框，离开时收起。浏览器核对浅色、深色、长草稿内滚动、切换布局保留附件和从列表中段保存。前端静态检查、全量测试和生产构建通过。 [功能说明](../../cat-two-product-development/references/product-guide.md#网页里的功能)
- **2026-11-22 00:03**　让桌面左侧看板独立滚动，标签笔记数随鼠标进入标签区或键盘聚焦显示。回收站接在最后一个标签下方，沿用 4px 行间距，移除分隔线。用 48 个标签核对滚动边界、计数显示、树形和平铺行间距、滚动条与宽度拖动。前端静态检查、全量测试和生产构建通过。 [功能说明](../../cat-two-product-development/references/product-guide.md#网页里的功能)
- **2026-11-22 00:09**　按确认的预览将首页「全部笔记」标题改为 17px、字重 700。浏览器核对浅色和深色的实际样式及标题菜单开合。前端静态检查、29 项相关测试和生产构建通过。 [标题菜单](../../../../web/src/components/MemoFeedMenu.tsx)
- **2026-11-22 00:20**　移除侧栏「新建备忘录」及其独立弹窗，从首页右侧吸顶输入框新建笔记。重新打开首页时优先恢复首页草稿，同账号的旧侧栏草稿和待上传附件也可恢复。浏览器核对草稿恢复顺序、滚动中保存及编辑器聚焦模式。前端静态检查、1,359 项测试和生产构建通过。 [功能说明](../../cat-two-product-development/references/product-guide.md#网页里的功能)
- **2026-11-22 00:28**　发布 Docker Hub `0.07`，核对编号、`latest`、提交标签及 AMD64 和 ARM64 两种平台。用正式镜像和独立 SQLite 卷验证新装、容器重建以及从 `0.06` 升级后的笔记读写。 [发布记录](development-log.md#2026-11-22上海时间)
- **2026-11-22 00:42**　将日历「视图选项」浮层改为按视口定位，消除两列笔记滚动时的瞬时位移，保留左侧独立滚动时的锚点跟踪。用 `0.07` 和修复后源码对照，检查滚轮事件和逐帧坐标、左栏滚动、排序选项及 Esc 返回焦点。57 项相关测试、前端静态检查和生产构建通过。 [日历视图选项](../../../../web/src/components/MemoDisplaySettingMenu.tsx)
- **2026-11-22 00:56**　将设置移到笔记页上方的独立弹窗，左侧选择分区，右侧单独滚动。旧 `/setting#分区` 链接登录后在首页打开对应分区。桌面浅色和深色核对草稿、筛选和笔记滚动位置、嵌套的 MCP 弹窗及关闭后的焦点。前端静态检查、1,361 项测试和生产构建通过。 [设置弹窗](../../../../web/src/components/Settings/SettingsDialog.tsx)
- **2026-11-22 01:13**　回收站保留进入前的日历和标签，点日期或标签回到对应列表。选中标签及回收站行统一用 `RGB(57, 115, 84)`。移除保存视图的界面、REST、Connect 和 MCP 工具，旧 `/views` 链接跳到首页，旧 SQLite 记录随升级保留。浅色和深色桌面验证侧栏滚动、标签选择、恢复笔记及归档返回。前端静态检查、1,342 项测试、生产构建及 SQLite 和服务端回归验证通过。 [功能说明](../../cat-two-product-development/references/product-guide.md#网页里的功能) · [API 变更](../../../../proto/api/CHANGELOG.md)
- **2026-11-22 01:21**　核对 Docker Hub `0.08`、`latest` 和提交标签及 AMD64、ARM64 两种平台。正式镜像通过从 `0.07` 升级的独立 SQLite 测试，旧笔记可读、新笔记可写，旧视图记录保留。Chromium 核对设置弹窗、回收站侧栏和标签颜色。 [发布记录](development-log.md#2026-11-22上海时间)
- **2026-11-22 09:50**　接入 Telegram Bot 私聊长轮询和企业微信自建应用加密回调，限定部署配置的发件人。文本写成私有笔记，附 `#Telegram` 或 `#微信`、来源标记及实时列表刷新。按消息 ID 去重，覆盖重复投递、已有编辑和回收站。模拟 Bot API、加密回调到 SQLite、服务端回归、竞态测试、静态检查和两份 Compose 校验通过。 [收件配置](../../cat-two-product-development/references/deployment.md#telegram-和企业微信收件)
- **2026-11-22 09:56**　Telegram 和企业微信成功保存笔记后回复 `Yes!`。Telegram 在原私聊回复，临时拒绝时限内重试一次，回复失败时继续收件。企业微信直接返回签名加密的确认。重复投递按消息 ID 去重。模拟 Bot API、企业微信回调到 SQLite、收件竞态测试和静态检查通过。 [收件说明](../../cat-two-product-development/references/deployment.md#telegram-和企业微信收件)
- **2026-11-22 10:20**　整理 AI 共创所用的 REST、MCP、文件和事件接口及凭据范围。修复会话并发轮换、PAT 撤销记录、附件匿名探测和企业微信握手权限，收紧请求体、代理信任和浏览器来源。更新前端依赖与 Alpine 运行层。SQLite、服务端竞态、前端构建和两份 Compose 配置验证通过。 [外部接口](../../cat-two-product-development/references/external-api.md) · [安全复盘](development-log.md#对外接口与公网加固)
- **2026-11-22 10:40**　检查匿名登录页面和相关接口的账号及凭据可见性。统一账号不存在与密码错误的 bcrypt 开销，限制登录与续期的浏览器来源，修正 REST 刷新 Cookie。连续 6 次密码失败持久封禁 IP，在「设置 → 安全」或部署主机解封。SQLite 迁移、重启、接口授权和 Chromium 六次失败到解封的流程通过，前端 1,350 项测试及生产构建通过。 [登录防护](../../cat-two-product-development/references/deployment.md#登录防护和-ip-解封)
- **2026-11-22 10:49**　核对 Docker Hub `0.09`、`latest` 和提交标签均对应 `1071616d`，包含 AMD64 与 ARM64。用正式 AMD64 镜像通过新装、容器重建及从 `0.08` 升级的 SQLite 笔记读写验证。 [发布记录](development-log.md#镜像-009)
- **2026-11-22 10:54**　合并 Docker 和 NAS 的 Compose。默认命名卷和 `PUID=1000`、`PGID=10`，支持自定义运行身份、沿用 `MEMOS_UID/GID` 及通过 `CAT_TWO_DATA_DIR` 绑定 NAS 原目录。用正式 `0.09` 镜像验证新装、自定义身份及两类旧部署重建后的笔记和附件。 [Compose](../../../../compose.yaml) · [切换说明](../../cat-two-product-development/references/deployment.md#备份和升级)
- **2026-11-22 11:04**　将 Compose 示例改为直接编辑镜像、端口、挂载和 PUID/PGID，补充中文注释，把反向代理和消息收件参数放到可选容器环境说明。用 `0.09` 验证默认命名卷、旧 NAS 路径和端口切换、笔记和附件读回，以及可选容器环境生效。同步更新工作流校验和部署文档。 [Compose](../../../../compose.yaml) · [切换说明](../../cat-two-product-development/references/deployment.md#备份和升级)
- **2026-11-22 11:34**　合并「存储」与「资源」，使用磁盘 Markdown 正文和本地附件，迁移旧 SQLite 和 S3 附件。验证 SQLite 迁移、失败重试、跨库隔离、附件权限和长中文文件名。前端 1,354 项测试和生产构建通过，用本地 AMD64 容器核对新装、从 `0.09` 升级及重建，用 Chromium 核对旧设置链接和统计展示。 [功能说明](../../cat-two-product-development/references/product-guide.md#数据备份和升级) · [备份和升级](../../cat-two-product-development/references/deployment.md#备份和升级)
- **2026-11-22 11:34**　复核网页登录、笔记创建和并发编辑、MCP 令牌及内部调用。记录登录页脚本、会话撤销、创建幂等、导入事务、修订号、MCP 权限与限流的修改方案和验收方式。 [代码审计](code-audit-2026-11-22.md)
- **2026-11-22 16:38**　落实登录、笔记和 MCP 审计修复。停用自定义脚本并启用 CSP，按会话撤销访问及刷新凭据。笔记创建支持幂等重试，编辑以修订号防止覆盖，ZIP 导入把置顶和归档并入保存事务。MCP 增加只读权限、鉴权前限流和批读上限，复用 Web、REST 和 Connect 的笔记服务。Chromium 验证两步登录及拦截内联脚本，本地 AMD64 镜像从 `0.09` 升级后验证旧 Cookie、笔记与附件读写和会话撤销。 [修复验证](code-audit-2026-11-22.md#修复与验证--2026-11-22) · [外部接口](../../cat-two-product-development/references/external-api.md)
- **2026-11-22 17:01**　新增管理员「通知」和「网络」设置。Telegram 与企业微信可独立配置和启停，首次保存前沿用相应容器环境。网络代理用于 Telegram 的轮询和回复，支持 HTTP、HTTPS、SOCKS5 和 SOCKS5H。保存后在运行中应用，凭据和代理 URL 经加密存入 SQLite，读取接口仅提供这些字段的配置状态。服务端竞态、SQLite、前端 1,365 项测试和生产构建通过。用 Chromium 和本地 AMD64 容器核对网页配置、代理认证、重建后的设置和企业微信加密回调写入笔记并回复 `Yes!`。 [配置说明](../../cat-two-product-development/references/deployment.md#telegram-和企业微信收件) · [设置接口](../../cat-two-product-development/references/external-api.md)
