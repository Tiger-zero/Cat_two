# Cat_two 镜像构建与发布

## 构建和发布镜像

在 GitHub Actions 打开 **Cat_two Container**，选择 `main` 和 `dockerhub` 并运行工作流。它使用仓库 Secrets `DOCKERHUB_USERNAME` 和 `DOCKERHUB_TOKEN`，验证后发布 AMD64 和 ARM64 镜像。推送到 `main` 的最新提交消息中包含 `[publish dockerhub]` 时也会触发发布。手动选择 `ghcr` 会将镜像推送到 `ghcr.io/tiger-zero/cat_two`。 [容器工作流](../../../../.github/workflows/cat-two-container.yml)

Docker Hub 版本从 `0.01` 起递增，每次发布提供编号、`latest` 和 `sha-<完整提交 SHA>` 三种标签。部署时可把 `compose.yaml` 中的 `image` 改为 `docker.io/tiger2896/cat_two:<版本号>` 来固定镜像。 [Docker Hub](https://hub.docker.com/r/tiger2896/cat_two) · [版本编号脚本](../../../../scripts/cat_two_docker_release.py)

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

## 验证发布

核对工作流结论、编号与 `latest` 标签、提交标签、两种平台及镜像提交。用独立数据卷验证新装、容器重建与 SQLite 数据读写。升级验证可以传入正式发布的旧镜像。 [冒烟脚本](../../../../scripts/release_smoke_test.sh) · [已有部署升级](../../cat-two-product-development/references/deployment.md#备份和升级)
