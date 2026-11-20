# Cat_two

Cat_two 是可以自行部署的个人笔记应用，基于 [Fumos](https://github.com/Tiger-zero/Fumos) 和 [Memos](https://github.com/usememos/memos)。后续开发以本仓库的 `main` 为基础。通过网页记录、整理和查找个人笔记。

## 部署服务端

在装有 Docker Compose 的服务器上克隆本仓库，进入仓库目录并运行以下命令。首次使用前请先构建或发布 Cat_two 镜像。

```sh
docker compose up -d
docker compose logs -f cat-two
```

在浏览器打开服务器的 5230 端口，按页面提示创建管理员账号。SQLite 数据和本地附件保存在 Compose 管理的 `cat_two_data` 卷中，容器内的挂载路径为 `/var/opt/memos`。升级前请备份这个卷。镜像更新后可以运行 `docker compose pull` 和 `docker compose up -d`。

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

## 在网页里管理笔记

- 在时间线创建笔记，可用 Markdown 写标题、清单、链接和代码片段。
- 用标签和空间整理主题，在搜索中按关键词找回内容。
- 按笔记设置可见范围，管理私人记录和需要共享的内容。

Cat_two 保留 Fumos 和 Memos 的 MIT 许可与来源记录。见 [LICENSE](LICENSE)。
