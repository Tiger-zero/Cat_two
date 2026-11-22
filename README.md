# Cat_two

Cat_two 是面向个人使用的自托管笔记应用。通过网页记录 Markdown 笔记和附件，按日期、标签或关键词找回内容。笔记仅限创建者登录后访问。默认使用 Docker Compose 和 SQLite，可部署在 NAS 上。界面默认是简体中文，网页时间和日历按上海时区显示。

[部署服务端](#部署服务端) · [在 NAS 上部署](#在-nas-上部署) · [Telegram 和企业微信收件](#telegram-和企业微信收件) · [备份和升级](#备份和升级) · [重置两步验证](#重置两步验证) · [连接 Codex](#连接-codex) · [功能与实现](.agents/skills/cat-two-development/references/product-guide.md)

## 部署服务端

在装有 Docker Compose 的机器上，将仓库中的 [compose.yaml](compose.yaml) 放到部署目录。在该目录运行以下命令，Compose 会从 Docker Hub 使用 `tiger2896/cat_two:latest`。

```sh
docker compose up -d
docker compose ps
```

在浏览器打开 `http://服务器地址:5230`。首次启动时按提示设置管理员用户名和密码，创建后网页注册关闭。需要查看启动日志时，运行 `docker compose logs -f cat-two`。

SQLite 和默认的本地附件存储使用 Compose 的 `cat_two_data` 命名卷，容器内路径为 `/var/opt/memos`。需要调整主机端口或容器时区时，可在项目环境中设置 `CAT_TWO_PORT` 或 `TZ`。

## 在 NAS 上部署

在绿联 NAS 上创建数据目录 `/volume1/docker/cat2/data`，让运行应用的用户和组有读写权限。将 [compose.nas.yaml](compose.nas.yaml) 导入 Docker 应用以创建项目。也可以在文件所在目录运行以下命令。

```sh
docker compose -f compose.nas.yaml up -d
```

NAS 配置默认将上述目录挂载到容器的 `/var/opt/memos`，以 `1000:10` 运行并映射到 NAS 的 `5230` 端口。使用其他目录、身份或端口时，在项目环境变量或 Compose 的 `.env` 中设置对应值。

```dotenv
PUID=1000
PGID=10
CAT_TWO_DATA_DIR=/volume1/docker/cat2/data
CAT_TWO_PORT=5230
```

普通 Compose 默认使用 `10001:10001`，也可通过 `PUID` 和 `PGID` 指定。两份配置都优先采用显式设置的 `MEMOS_UID` 和 `MEMOS_GID`。

## 在网页里管理笔记

- 在「日历」中写笔记，也可以选择左侧日期，只看当天的笔记。笔记支持 Markdown 和附件。
- 用标签整理内容，用搜索找回笔记。「全部笔记」菜单可调整排序、筛选来源和多选导出。网页新建笔记标记为「网页端」。
- 在「设置 → 偏好设置 → 语言」切换简体中文或 English，在「设置 → 系统」修改管理员密码。

## 连接 Codex

在 Cat_two 的「设置 → MCP 和 API」创建专用 MCP 连接令牌，可选 7 天（默认）、14 天、1 年或永久有效。在本地 Codex 添加 Streamable HTTP 服务，粘贴 `/mcp` 地址和完整的 `Authorization` 请求头值，Bearer 令牌环境变量留空。令牌创建后即可连接。可选的 Cat_two 笔记插件提供整理待办和归档任务的工作流。[接入步骤](plugins/cat-two-notes/README.md#连接-nas) · [可选插件安装](plugins/cat-two-notes/README.md#安装笔记插件)

设置页显示首次连接的客户端 IP 和令牌到期时间。点击「删除令牌」并确认后，客户端的下一次 MCP 请求会被拒绝。升级前签发的有效令牌沿用原到期时间。REST 集成使用个人访问令牌（PAT）。

## Telegram 和企业微信收件

先完成 Cat_two 的管理员初始化。在 NAS 的 Compose 项目环境变量或部署目录下的 `.env` 中按需配置以下渠道，然后用原项目和数据挂载重建容器。收件目前支持文本消息，包括用文本发来的链接。消息成为管理员的私有笔记，分别带 `#Telegram` 或 `#微信` 标签，可在「全部笔记」和标签列表查看。每个渠道只收指定发件人的消息。令牌和加密密钥只填在部署配置里。

笔记保存成功后，Telegram Bot 或企业微信应用会在原会话回复 `Yes!`。被忽略或保存失败的消息不会收到确认。若渠道暂时无法投递确认，已保存的笔记仍可在 Cat_two 查看。

### Telegram Bot

1. 在 Telegram 向 [@BotFather](https://t.me/BotFather) 发送 `/newbot`，创建专用于 Cat_two 的 Bot。打开与这个 Bot 的私聊，发送一条文本消息。
2. 取得本人 Telegram 数字 ID。可在启动 Cat_two 轮询前调用官方 Bot API 的 `getUpdates`，查看该私聊消息的 `message.from.id`。Bot Token 和 Telegram 用户名都与这个数字 ID 不同。 [Telegram Bot API](https://core.telegram.org/bots/api#getupdates)
3. 设置下面两个变量并重建容器。NAS 需要能主动访问 `https://api.telegram.org`。为这个 Bot 保留一个轮询进程，已有 webhook 的 Bot 需要先解除 webhook 才能用轮询。 [Telegram 轮询规则](https://core.telegram.org/bots/api#getupdates)

```dotenv
CAT_TWO_TELEGRAM_BOT_TOKEN=填BotFather给你的令牌
CAT_TWO_TELEGRAM_USER_ID=填你的数字ID
```

<details>
<summary>用官方 Bot API 查询自己的 Telegram 数字 ID</summary>

先给新 Bot 发一条私聊文本。然后在能访问 Telegram 的电脑上运行下面的命令，按提示输入 Bot Token，终端会列出私聊发件人的数字 ID 和名称。

```sh
python3 - <<'PY'
import getpass
import json
import re
import urllib.request

token = getpass.getpass("BotFather token: ").strip()
if not re.fullmatch(r"[0-9]+:[A-Za-z0-9_-]+", token):
    raise SystemExit("Bot Token 格式错误")
try:
    with urllib.request.urlopen(f"https://api.telegram.org/bot{token}/getUpdates", timeout=10) as response:
        updates = json.load(response)["result"]
except (OSError, ValueError, KeyError):
    raise SystemExit("无法获取更新。请检查令牌、网络以及该 Bot 是否配置了 webhook。") from None
for update in updates:
    message = update.get("message", {})
    if message.get("chat", {}).get("type") == "private" and message.get("from"):
        sender = message["from"]
        print(sender["id"], sender.get("username") or sender.get("first_name", ""))
PY
```

</details>

### 企业微信自建应用

1. 在企业微信管理后台的「应用管理 → 自建」创建应用，把自己加入可见范围。记录企业 ID、应用的 AgentId 和通讯录中的成员账号 UserID。
2. 在应用的「接收消息」里生成 Token 和 EncodingAESKey。将以下五项填入 Cat_two 的项目配置并重建容器。收件和加密回复使用现有回调配置，无需应用 Secret。
3. 将公网 HTTPS 地址的 `/integrations/wecom` 路径通过反向代理转发到 Cat_two 的 `5230` 端口，并填入企业微信「接收消息」的 URL，例如 `https://notes.example.com/integrations/wecom`。保存时平台会发起验证请求。NAS 只有局域网地址时，需要有公网可达的反向代理入口。
4. 在企业微信客户端打开应用并发送文本。若企业后台提供「我的企业 → 微信插件」，可用个人微信扫码关注，再从微信里打开该企业应用发消息。 [企业微信回调配置](https://developer.work.weixin.qq.com/document/path/90930) · [接收消息](https://developer.work.weixin.qq.com/document/path/90968) · [Media Saber 设置参考](https://wiki.msaber.fun/usage/system_settings/message/weixin.html)

```dotenv
CAT_TWO_WECOM_CORP_ID=填企业ID
CAT_TWO_WECOM_AGENT_ID=填应用AgentId
CAT_TWO_WECOM_CALLBACK_TOKEN=填接收消息页面的Token
CAT_TWO_WECOM_ENCODING_AES_KEY=填43位EncodingAESKey
CAT_TWO_WECOM_USER_ID=填你的企业微信成员账号
```

使用普通 Compose 时运行 `docker compose up -d`，使用 NAS 配置时运行 `docker compose -f compose.nas.yaml up -d`。查看对应项目的 `cat-two` 容器日志，可确认渠道是否启用。若企业微信回调验证失败，先核对 HTTPS 回调路径、Token、EncodingAESKey 和 NAS 系统时间。

## 两步验证

进入「设置 → 我的账户」，使用当前密码获取设置密钥。将密钥手动添加到验证器应用，再输入六位动态码完成绑定。绑定后需要重新登录，请使用验证器生成的下一组动态码。

### 重置两步验证

换手机或丢失验证器后，有 Docker 管理权限的人可以在运行中的容器内重置。容器需要运行包含两步验证功能的镜像，并完成启动。命令中的 `admin` 请换成网页登录时使用的用户名。

**绿联 NAS 容器终端**

打开 NAS 的 Docker 应用，进入正在运行的 Cat_two 容器的终端，执行以下命令。

```sh
/usr/local/memos/memos two-factor reset --username admin
```

**从 Docker 宿主机执行**

在部署目录内执行对应命令。沿用启动时的 Compose 文件、项目名称和环境文件。

使用普通 Compose 时运行以下命令。

```sh
docker compose exec cat-two /usr/local/memos/memos two-factor reset --username admin
```

使用仓库 NAS 配置时运行以下命令。

```sh
docker compose -f compose.nas.yaml exec cat-two /usr/local/memos/memos two-factor reset --username admin
```

看到 `Two-factor authentication reset. Sign in again.` 表示重置成功。命令会关闭该账号的两步验证并注销旧浏览器会话。用原密码登录后，可重新绑定验证器。若终端提示找不到 `two-factor` 子命令，请先更新到包含两步验证的镜像。 [恢复命令实现](cmd/memos/two_factor.go)

## 备份和升级

升级前先停止容器并备份整个数据目录或命名卷，也可以使用存储的一致性快照。使用其他目录或对象存储存放附件时，一并备份。升级时沿用部署时的 Compose 配置和项目名称，继续挂载存有现有 SQLite 数据和附件的卷或目录。使用 NAS 配置时，给以下两条命令都加上 `-f compose.nas.yaml`。

```sh
docker compose pull
docker compose up -d
```

若通过 `CAT_TWO_IMAGE` 固定了镜像标签，拉取前请把标签改为希望升级到的版本。想迁移笔记时，可在「设置 → 导出和导入」导出 ZIP。 [数据备份与恢复](.agents/skills/cat-two-development/references/product-guide.md#数据备份和升级)

## 构建和发布镜像

在 GitHub Actions 打开 **Cat_two Container**，选择 `main` 和 `dockerhub` 并运行工作流。它使用仓库 Secrets `DOCKERHUB_USERNAME` 和 `DOCKERHUB_TOKEN`，验证后发布 AMD64 和 ARM64 镜像。也可以在推送到 `main` 的最新提交消息加入 `[publish dockerhub]` 触发发布。手动选择 `ghcr` 时，镜像发布到 `ghcr.io/tiger-zero/cat_two`。 [容器工作流](.github/workflows/cat-two-container.yml)

Docker Hub 版本从 `0.01` 起，每次发布递增 `0.01`，例如 `0.09` 的下一版是 `0.10`。每次发布同时提供编号、`latest` 和 `sha-<完整提交 SHA>` 三种标签。要固定版本，在 Compose 环境中把 `CAT_TWO_IMAGE` 设为 `docker.io/tiger2896/cat_two:<版本号>`。 [Docker Hub 镜像](https://hub.docker.com/r/tiger2896/cat_two)

也可以在独立检出目录构建本地镜像。先用 pnpm 构建前端，再让 Docker 将静态文件嵌入服务端。

```sh
(cd web && pnpm install --frozen-lockfile && pnpm release)
docker build -f scripts/Dockerfile \
  --build-arg VERSION="$(bash scripts/release_version.sh development-version)" \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  -t cat-two:local .
CAT_TWO_IMAGE=cat-two:local docker compose up -d
```

## 项目来源与许可

Cat_two 基于 Fumos 和 Memos，保留原项目的 MIT 许可与来源记录。后续开发以本仓库的 `main` 为基础。 [Fumos](https://github.com/Tiger-zero/Fumos) · [Memos](https://github.com/usememos/memos) · [MIT 许可](LICENSE)
