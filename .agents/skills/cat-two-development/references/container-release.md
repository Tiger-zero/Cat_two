# Cat_two 镜像构建与发布

## 构建和发布镜像

在 GitHub Actions 打开 **Cat_two Container**，选择 `main` 和 `dockerhub` 并运行工作流。Docker Hub 使用仓库 Secrets `DOCKERHUB_USERNAME` 和 `DOCKERHUB_TOKEN`。推送到 `main` 的最新提交消息中包含 `[publish dockerhub]` 时也会触发发布。手动选择 `ghcr` 会将镜像推送到 `ghcr.io/tiger-zero/cat_two`。 [容器工作流](../../../../.github/workflows/cat-two-container.yml)

发布流程核对同一提交的后端、前端和 Proto 检查，数据库验证统一使用 SQLite。按镜像摘要分别在 Linux AMD64 和 ARM64 上使用全新数据卷验证候选镜像，随后用同一镜像和数据卷重建容器。通过后才更新发布标签。Docker Hub 版本从 `0.01` 起递增，提供编号、`latest` 和 `sha-<完整提交 SHA>` 三种标签。GHCR 提供 `latest` 和提交标签。部署时可把 `compose.yaml` 中的 `image` 改为 `docker.io/tiger2896/cat_two:<版本号>` 来固定镜像。 [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two) · [版本编号脚本](../../../../scripts/cat_two_docker_release.py)

## 本地构建

在独立检出目录使用 Node.js `24.22.3` 或更新的 24.x 版本。pnpm 版本见 `web/package.json`。先构建前端，再将静态文件嵌入 Docker 镜像。 [CI 工具链](../../../../.github/actions/setup-web/action.yml)

```sh
(cd web && pnpm install --frozen-lockfile && pnpm release)
docker build -f scripts/Dockerfile \
  --build-arg VERSION="$(bash scripts/release_version.sh development-version)" \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  -t cat-two:local .
```

把独立测试项目的 `compose.yaml` 中的 `image` 改为 `cat-two:local`，执行 `docker compose up -d`。 [项目 Compose](../../../../compose.yaml)

单独修改 Web 测试或说明时，容器工作流跳过镜像构建。本地冒烟未传入候选镜像时，会在临时目录准备 Docker 上下文和前端产物。跨 runner 可恢复 Go 模块下载层，编译阶段离线运行。

## 验证发布

核对工作流结论及两种平台的镜像提交。Docker Hub 的编号、`latest` 和对应提交标签应指向同一镜像索引。冒烟脚本使用网页的 Connect 登录和会话刷新，创建笔记并上传附件，重建容器后核对笔记、附件字节和回收站恢复。候选镜像的旧 `/api` 路由应返回 HTTP 404。 [冒烟脚本](../../../../scripts/release_smoke_test.sh)

```sh
bash scripts/release_smoke_test.sh \
  --candidate-image cat-two:local \
  --expected-commit "$(git rev-parse HEAD)"
```
