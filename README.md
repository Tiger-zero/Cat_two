# Cat_two

Cat_two 是可以自行部署的个人笔记应用，基于 [Fumos](https://github.com/Tiger-zero/Fumos) 和 [Memos](https://github.com/usememos/memos)。后续开发以本仓库的 `main` 为基础。通过网页记录、整理和查找个人笔记。

## 部署服务端

在装有 Docker Compose 的服务器上克隆本仓库，进入仓库目录并运行以下命令。首次使用前请先构建或发布 Cat_two 镜像。

```sh
docker compose up -d
docker compose logs -f cat-two
```

在浏览器打开服务器的 5230 端口，按页面提示创建管理员账号。SQLite 数据和本地附件保存在 Compose 管理的 `cat_two_data` 卷中，容器内的挂载路径为 `/var/opt/memos`。升级前请备份这个卷。镜像更新后可以运行 `docker compose pull` 和 `docker compose up -d`。镜像和 Compose 默认使用 `Asia/Shanghai` 时区，可用 `TZ` 覆盖。

在 GitHub Actions 中打开 **Cat_two Container**，选择 `main` 和 `dockerhub` 并运行工作流。它使用仓库的 `DOCKERHUB_USERNAME` 和 `DOCKERHUB_TOKEN` Secrets 登录 Docker Hub。通过启动验证后，工作流会将 AMD64 和 ARM64 镜像发布到 `docker.io/tiger2896/cat_two`，标签为 `latest` 和 `sha-<完整提交 SHA>`。也可在推送到 `main` 的最新提交消息中加上 `[publish dockerhub]` 来触发发布。手动选择 `ghcr` 时，目标为 `ghcr.io/tiger-zero/cat_two`。

也可以在单独的干净检出目录里构建本地镜像。前端发布命令会生成嵌入 Go 程序的静态文件。

```sh
(cd web && pnpm install --frozen-lockfile && pnpm release)
docker build -f scripts/Dockerfile \
  --build-arg VERSION="$(bash scripts/release_version.sh development-version)" \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  -t cat-two:local .
CAT_TWO_IMAGE=cat-two:local docker compose up -d
```

## 在 NAS 上部署

绿联 NAS 可以导入 [compose.nas.yaml](compose.nas.yaml)，也可以从仓库目录运行：

```sh
docker compose -f compose.nas.yaml up -d
```

这份配置把 NAS 上的 `/volume1/docker/cat2/data` 挂载到容器的 `/var/opt/memos`，应用默认以 `1000:10` 运行，网页端口为 `5230`。首次部署需先创建该目录并授予运行身份读写权限。已有 NAS 实例应继续使用存放 SQLite 数据和附件的原路径，备份后再更新。如果现有实例使用普通 Compose 的 `cat_two_data` 命名卷，请继续用原配置升级。NAS 配置会读取指定目录中的数据。

如果本机目录、运行身份或端口不同，可以在 NAS 项目的环境变量中设置对应值。也可以放在 Compose 使用的 `.env` 文件中：

```dotenv
PUID=1000
PGID=10
CAT_TWO_DATA_DIR=/volume1/docker/cat2/data
CAT_TWO_PORT=5230
```

两份 Compose 都会把项目的 `PUID/PGID` 转成已发布镜像也支持的 `MEMOS_UID/MEMOS_GID`。显式设置的 `MEMOS_UID/MEMOS_GID` 优先。普通 Compose 的默认运行身份为 `10001:10001`。

## 在网页里管理笔记

界面默认使用简体中文，可在「设置 → 偏好设置 → 语言」切换为 English。网页的时间和日历按上海时区显示。

- 在时间线创建笔记，可用 Markdown 写标题、清单、链接和代码片段。
- 用标签和空间整理主题，在搜索中按关键词找回内容。
- 按笔记设置可见范围，管理私人记录和需要共享的内容。

Cat_two 保留 Fumos 和 Memos 的 MIT 许可与来源记录。见 [LICENSE](LICENSE)。
