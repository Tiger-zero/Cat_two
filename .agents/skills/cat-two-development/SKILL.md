---
name: cat-two-development
description: 在 Cat_two 仓库开发、排查缺陷、验证升级和交付修改。涉及网页体验、个人笔记、数据库迁移、NAS 部署、Docker 镜像或开发中断后恢复时使用。
---

# Cat_two 开发

## 产品和代码入口

Cat_two 从 Fumos 和 Memos 衍生而来，以 `Tiger-zero/Cat_two` 的 `main` 为日常开发分支。当前交付是 Docker 服务和网页，默认使用 SQLite，适合在 NAS 上管理个人笔记。产品现状和部署方式分别见 [功能与实现](../../../docs/cat-two-guide.md) 和 [README](../../../README.md)。

- 网页和主题在 `web/`，API 和文件服务在 `server/`，数据和迁移在 `store/`，接口定义在 `proto/`。按 [仓库开发规范](../../../AGENTS.md) 复用现有调用路径和测试。
- 默认简体中文，仅提供简体中文和英语。日历、笔记日期和日期筛选统一使用 `Asia/Shanghai`。调整局部颜色或字体时，先找实际渲染组件和主题变量，再核对相邻状态与主题。
- 个人笔记的权限由服务端检查。移除入口时一起核对路由、设置、API 和数据依赖。升级时保留旧有笔记、附件及关联数据，使原作者仍能访问自己的内容。

## 2026 年 11 月 20 日至 21 日的开发经验

日期按上海时区记录，提交可用于回看变更动机。

| 日期 | 当时的工作 | 后续开发应核对 |
| --- | --- | --- |
| 11 月 20 日 | 从 Fumos 导入基线，建立 GitHub Actions 容器构建和 Docker Hub 发布。修复 NAS 数据目录的 UID/GID 配置，把健康检查固定在容器回环地址。 [NAS 权限提交](https://github.com/Tiger-zero/Cat_two/commit/ddba0c6e7) | 复用原数据挂载。出现启动失败时先区分数据目录权限、进程启动和 HTTP 探测。普通与 NAS 两份 Compose 都需要与实际镜像的环境变量匹配。 |
| 11 月 20 日至 21 日 | 改为中文和上海时间，调整界面、字形和猫图标。用「日历」承接原时间线及按日筛选，移除地图、独立附件库和独立日历页。 [日历与附件修正](https://github.com/Tiger-zero/Cat_two/commit/e822b27de) | 回放「点日期 → 新建笔记 → 清除筛选 → 返回该日期」，核对有笔记及无笔记日期、日期边界、旧链接和笔记内附件预览。 |
| 11 月 21 日 | 加入回收站、标签置顶和目录重命名、来源筛选及外部接入，修复历史升级测试夹具。 [功能提交](https://github.com/Tiger-zero/Cat_two/commit/ca1b0c88f) · [夹具修正](https://github.com/Tiger-zero/Cat_two/commit/c472d1bf4) | 核对移入和恢复后的状态及附件。标签重命名到已有同级标签时应合并两边笔记，并迁移子路径。迁移用例要按旧版本当时的表和列准备数据，迁移后再检查新结构。 |
| 11 月 21 日 | 首次创建管理员后关闭网页注册，停用空间等设置模块，加入可选的验证器两步验证和容器终端恢复。 [个人模式和两步验证](https://github.com/Tiger-zero/Cat_two/commit/51bfcff31) | 同时测试首次安装和旧数据升级。核对旧空间笔记的作者权限、启用后的动态码登录和防重放，以及终端重置后的旧会话失效和原密码登录。 |
| 11 月 21 日 | 建立 Docker Hub 编号规则，保留许可，并将重置两步验证的命令写入 README。 [编号规则提交](https://github.com/Tiger-zero/Cat_two/commit/b33aa59d5) · [README 提交](https://github.com/Tiger-zero/Cat_two/commit/4232a6a58) | 分别核对源码提交、Docker Hub 镜像和 NAS 正在运行的版本。README 中的命令须匹配对应镜像和 Compose 文件。 |

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
