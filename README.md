# Cat_two

Cat_two 是面向个人使用的自托管笔记应用。通过网页记录 Markdown 笔记和附件，按日期、标签或关键词找回内容。笔记仅限创建者登录后访问。默认使用 Docker Compose 和 SQLite，可部署在 NAS 上。界面默认是简体中文，网页时间和日历按上海时区显示。

[部署服务端](#部署服务端) · [在 NAS 上部署](#在-nas-上部署) · [在公网部署](#在公网部署) · [登录防护和 IP 解封](#登录防护和-ip-解封) · [Telegram 和企业微信收件](#telegram-和企业微信收件) · [Telegram 网络代理](#telegram-网络代理) · [备份和升级](#备份和升级) · [重置两步验证](#重置两步验证) · [连接 Codex](#连接-codex) · [外部接口](.agents/skills/cat-two-product-development/references/external-api.md) · [功能与实现](.agents/skills/cat-two-product-development/references/product-guide.md)

## 部署服务端

在装有 Docker Compose 的机器上，将仓库中的 [compose.yaml](compose.yaml) 放到部署目录。在该目录运行以下命令，Compose 会从 Docker Hub 使用 `tiger2896/cat_two:latest`。

```sh
docker compose up -d
```

在浏览器打开 `http://服务器地址:5230`。首次启动时按提示设置管理员用户名和密码，创建后网页注册关闭。用 `docker compose ps` 查看运行状态，或用 `docker compose logs -f cat-two` 查看启动日志。

默认将 `cat_two_data` 命名卷挂载到容器的 `/var/opt/memos`。Markdown 正文保存在 `notes/`，新附件保存在 `assets/`，SQLite 管理账户、笔记元数据和搜索索引。「设置 → 存储」可查看存放位置和使用量。如需自定义部署，直接编辑 `compose.yaml` 中的 `image`、`ports` 或 `environment`。运行身份由 `PUID` 和 `PGID` 指定，默认是 `1000` 和 `10`，时区由 `TZ` 指定。

## 在 NAS 上部署

NAS 使用同一份 `compose.yaml`。新部署可将文件导入绿联 NAS 的 Docker 应用，也可在文件所在目录运行 `docker compose up -d`。如果想把数据直接放到 NAS 文件夹，先创建目录并赋予应用的运行用户和组读写权限，再将 `cat-two` 服务下 `volumes` 的挂载替换为下面这一条。需要用其他目录时，修改冒号前的宿主机绝对路径。

```yaml
volumes:
  - /volume1/docker/cat2/data:/var/opt/memos
```

已有实例应先核对原项目的端口、数据挂载和运行身份，再替换配置。 [备份和升级](#备份和升级)

## 可选的容器环境变量

公网反向代理的配置填在 Docker 或 NAS 管理界面中 `cat-two` 服务的容器环境变量里。命令行部署可把所需变量写入私有的 `.env.integrations` 文件，使用 `变量名=值` 格式，然后在 `cat-two` 服务下加入这一行，与 `environment` 同级。

```yaml
env_file: ./.env.integrations
```

含有令牌或密钥的文件仅保存在部署主机。保存配置后，用原项目执行 `docker compose up -d` 重建容器。

## 在公网部署

首次启动时先通过受限网络或 SSH 转发访问 `5230`，完成唯一管理员的创建，设置独立密码并在「设置 → 我的账户」启用两步验证。代理与 Cat_two 在同一主机时，将 `ports` 的映射改为 `127.0.0.1:5230:5230`，并在 `cat-two` 容器环境中将 `MEMOS_INSTANCE_URL` 设为实际的公网 HTTPS 地址，例如 `https://notes.example.com`。 [容器环境变量](#可选的容器环境变量)

在反向代理上终止 HTTPS，转发到 `http://127.0.0.1:5230`，之后用配置的 HTTPS 域名登录。防火墙只开放代理的 HTTPS 端口及签发证书所需的 HTTP 端口，HTTP 入口重定向到 HTTPS。代理部署在 Docker 网络内时，可让代理在专用网络里访问容器，并移除 `cat-two` 的端口发布。

- 代理校验公网域名、保留 `Host`、设置 `X-Forwarded-Proto`，并在公网入口覆盖客户端传入的 `X-Forwarded-For` 和 `X-Real-IP`。Cat_two 默认从直连地址识别客户端。在 `cat-two` 容器环境中将 `MEMOS_TRUSTED_PROXIES` 设为应用实际收到连接的代理 IP 或专用代理网段，以便按真实客户端区分限流和 MCP 连接来源。
- 同一个 SQLite 实例由一个 Cat_two 进程提供服务，以共享会话更新和限流状态。保留应用限流，并在代理侧限制登录和首次设置请求频率、请求体大小及慢上传。大附件和 ZIP 用分块接口。`/api/sse` 须关闭代理响应缓冲，给流式连接留出读取时间。代理和 CDN 应跳过 `/api/`、`/memos.api.*`、`/mcp`、`/file/` 和 `/integrations/` 的缓存。
- 仅需移动端收件时，企业微信只需公网代理 `/integrations/wecom`，Telegram 通过 NAS 或服务器主动连接 Bot API。其他入口可放在 VPN 或受限网络中。
- 给每个集成单独创建短期令牌，撤销闲置或已经泄露的令牌。公网日志避免记录 `Authorization`、Cookie 和企业微信回调查询参数。备份 SQLite、附件和部署密钥时同样限制访问。 [凭据和权限](.agents/skills/cat-two-product-development/references/external-api.md#凭据和可见性) · [安全说明](SECURITY.md)

## 在网页里管理笔记

- 在「日历」中写笔记，也可以选择左侧日期，只看当天的笔记。笔记支持 Markdown 和附件。
- 在编辑器点 `#` 或直接输入 `#读书/随笔`。多个标签用空格分开。侧栏选择父标签会查找整个分支，搜索面板的「无标签」可找出尚未分类的笔记。
- 标签菜单可以重命名分支。将目标填为 `#读书/随笔` 可以移动到其他父标签。删除时可仅移除标签并保留笔记，也可把匹配的笔记移入回收站。 [标签说明](.agents/skills/cat-two-product-development/SKILL.md#标签)
- 「全部笔记」菜单可调整排序、筛选来源和多选导出。网页新建笔记标记为「网页端」。
- 通过侧栏切换浅色和深色，也可以在「设置 → 偏好设置」选择纸张或跟随系统。 [桌面设计与配色](.agents/skills/cat-two-ui-development/references/desktop-visual-design.md)
- 在「设置 → 偏好设置 → 语言」切换简体中文或 English，在「设置 → 系统」修改管理员密码。

## 连接 Codex

在 Cat_two 的「设置 → MCP 和 API」创建专用 MCP 连接令牌，可选 7 天（默认）、14 天、1 年或永久有效。权限默认读写，也可选只读用于搜索和查询。使用本地 Codex 时添加 Streamable HTTP 服务，粘贴 `/mcp` 地址和完整的 `Authorization` 请求头值，Bearer 令牌环境变量留空。令牌创建后即可连接。可选的 Cat_two 笔记插件提供整理待办和归档任务的工作流。 [接入步骤](plugins/cat-two-notes/README.md#连接-nas) · [可选插件安装](plugins/cat-two-notes/README.md#安装笔记插件)

设置页显示令牌权限、首次连接的客户端 IP 和到期时间。点击「删除令牌」并确认后，客户端的下一次 MCP 请求会被拒绝。升级前签发的有效令牌按原到期时间获得读写权限。REST 集成使用个人访问令牌（PAT）。

## Telegram 和企业微信收件

完成管理员初始化后，在「设置 → 通知」分别启用并保存 Telegram 和企业微信。保存后立即应用，容器重建时用同一个数据卷即可保留设置。收件支持文本和文本链接。每个渠道只收指定发件人的消息，保存为管理员的私有笔记，并添加 `#Telegram` 或 `#微信` 标签。

笔记保存成功后，Telegram Bot 或企业微信应用会在原会话回复 `Yes!`。被忽略或保存失败的消息不会收到确认。若渠道暂时无法投递确认，已保存的笔记仍可在 Cat_two 查看。

已有部署的收件环境变量会作为各渠道首次保存前的配置。网页设置按渠道优先，包括停用状态。令牌和密钥加密保存，之后修改时可留空以保留原值。

### Telegram Bot

1. 在 Telegram 向 [@BotFather](https://t.me/BotFather) 发送 `/newbot`，创建专用于 Cat_two 的 Bot。打开与这个 Bot 的私聊，发送一条文本消息。
2. 取得本人 Telegram 数字 ID。可在启动 Cat_two 轮询前调用官方 Bot API 的 `getUpdates`，查看该私聊消息的 `message.from.id`。Bot Token 和 Telegram 用户名都与这个数字 ID 不同。 [Telegram Bot API](https://core.telegram.org/bots/api#getupdates)
3. 在「设置 → 通知 → Telegram」填写 Bot Token 和数字用户 ID，启用并保存。容器访问 Telegram 受限时，在「设置 → 网络」填入代理。已有 webhook 的 Bot 需先解除 webhook，再由 Cat_two 单独轮询。 [Telegram 网络代理](#telegram-网络代理) · [Telegram 轮询规则](https://core.telegram.org/bots/api#getupdates)
4. 给 Bot 发送一条新的私聊文本，看到 `Yes!` 即表示笔记已保存。

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
2. 在企业微信后台打开应用的「接收消息 → 设置 API 接收」，生成 Token 和 EncodingAESKey。在 Cat_two「设置 → 通知 → 企业微信」填写对应凭据和公网回调地址，启用并保存。填 `notes.example.com:8443` 会生成 `https://notes.example.com:8443/integrations/wecom`。也可填写完整回调 URL。企业微信支持 HTTP 和 HTTPS，推荐 HTTPS。 [接收消息与事件](https://developer.work.weixin.qq.com/document/path/90238)
3. 让公网入口将回调的 GET 和 POST（含查询参数）转发到 Cat_two 的 `/integrations/wecom`。NAS 在局域网时，可通过公网反向代理回源到容器的 `5230` 端口。复制 Cat_two 已保存的完整 URL 到企业微信后台，与相同的 Token 和 EncodingAESKey 一起保存。企业微信会发出签名加密的 GET 验证，服务须在 1 秒内返回解密后的原文。 [回调配置](https://developer.work.weixin.qq.com/document/path/90930) · [加解密方案](https://developer.work.weixin.qq.com/document/path/90968)
4. 在企业微信客户端打开应用并发送文本。若企业后台提供「我的企业 → 微信插件」，可用个人微信扫码关注，再从微信里打开该企业应用发消息。 [Media Saber 设置参考](https://wiki.msaber.fun/usage/system_settings/message/weixin.html)

手动地址优先于 `MEMOS_INSTANCE_URL` 提供的回调地址。清空后保存可恢复实例级默认值。实例只有内网 IP 时，先填写公网域名再复制。验证失败时检查公网端口、反向代理路由、Token、EncodingAESKey 和 NAS 系统时间。

### Telegram 网络代理

在「设置 → 网络」启用代理，填入容器可访问的完整 URL，例如 `http://192.168.1.10:7890` 或 `socks5://192.168.1.10:1080`。支持 `http`、`https`、`socks5` 和 `socks5h`。代理需要认证时可使用 `http://用户名:密码@代理地址:端口` 的格式，账户或密码中的保留字符应做 URL 编码。代理在另一台设备上时，使用该设备的局域网 IP 并确认代理端口对容器可达。

保存后 Telegram 的轮询和回复马上改用设置的代理。首次保存前沿用容器的 `HTTPS_PROXY` 和 `NO_PROXY`。手动关闭并保存后，Telegram 直接连接。企业微信由平台请求 Cat_two 的公网 HTTPS 回调地址。

## 登录防护和 IP 解封

同一 IP 连续 6 次密码验证失败时会被禁止发起新的登录。不存在的账号也会计入该 IP 的连续失败次数。完成一次登录会清除尚未封禁的失败记录。封禁持久保存，重启后仍然有效，直到管理员手动解除。共享同一公网 IP 的设备会受到同一条封禁。反向代理后部署时，按公网部署说明配置可信代理，让应用能区分客户端 IP。

已登录的管理员可以在「设置 → 安全 → 黑名单 IP」查看封禁时间并解除。若当前所有浏览器都无法登录，可在部署主机上用正在运行的 Compose 项目解封指定地址。将示例 IP 换成实际被封禁的地址，应用日志中会记录该地址。

```sh
docker compose exec cat-two /usr/local/memos/memos login-blocklist remove --ip 203.0.113.42
```

执行时沿用正在运行的 Compose 项目名称和环境文件。

## 两步验证

进入「设置 → 我的账户」，使用当前密码获取设置密钥。将密钥手动添加到验证器应用，再输入六位动态码完成绑定。绑定后需要重新登录，请使用验证器生成的下一组动态码。

每次登录生成独立会话。退出或从「我的账户」撤销会话后，同一会话的访问和刷新凭据立即失效。升级后，浏览器会用有效的刷新 Cookie 将旧会话升级为新格式。旧 Cookie 已过期或被撤销时，需要重新登录。

网页登录后连续 45 分钟没有编辑笔记，页面会要求重新登录。输入和修改正文、调整格式、增删附件或勾选笔记中的待办都会重新计时。只读浏览和后台同步不计入编辑活动。同一浏览器会话的多个标签页共用计时，重新登录后可以继续当前浏览器中已保存的草稿。升级时，尚无编辑记录的旧会话按现有凭据的签发时间计时，满 45 分钟需重新登录。REST 个人访问令牌和 MCP 连接令牌按各自有效期运行。

### 重置两步验证

换手机或丢失验证器后，有 Docker 管理权限的人可以在运行中的容器内重置。容器需要运行包含两步验证功能的镜像，并完成启动。命令中的 `admin` 请换成网页登录时使用的用户名。

**绿联 NAS 容器终端**

打开 NAS 的 Docker 应用，进入正在运行的 Cat_two 容器的终端，执行以下命令。

```sh
/usr/local/memos/memos two-factor reset --username admin
```

**从 Docker 宿主机执行**

在部署目录内执行以下命令，沿用启动时的项目名称和环境文件。

```sh
docker compose exec cat-two /usr/local/memos/memos two-factor reset --username admin
```

看到 `Two-factor authentication reset. Sign in again.` 表示重置成功。命令会关闭该账号的两步验证并注销旧浏览器会话。用原密码登录后，可重新绑定验证器。若终端提示找不到 `two-factor` 子命令，请先更新到包含两步验证的镜像。 [恢复命令实现](cmd/memos/two_factor.go)

## 备份和升级

升级前先停止容器并备份整个数据目录或命名卷，也可以使用存储的一致性快照。恢复时将同一份备份中的 SQLite、`notes/` 和 `assets/` 一起还原。旧实例使用独立目录或对象存储时，把原附件一起纳入升级备份。

首次启动新版本时，应用会将旧笔记正文及以前保存在 SQLite 或 S3 的附件迁入文件系统。旧 S3 的服务地址、对象和读取凭据在迁移完成前仍需可用。已有本地附件会沿用原位置，使用独立目录的部署需继续挂载这些目录。若启动报迁移错误，先恢复原附件的读取权限或连接，再用同一份数据重试。若要回退到旧镜像，请恢复升级前的备份。

Markdown 文件由应用管理，直接编辑磁盘文件会导致校验失败。笔记替换或彻底删除后，已无引用的旧正文文件在下次启动时清理。回收站里的笔记仍保留其正文和附件。

替换 Compose 配置时，沿用原项目名称和数据挂载。简化后的示例直接使用文件中填写的值，旧 `.env` 中的应用部署参数需要在重建前手动迁移。

- 原实例绑定 NAS 目录时，先核对旧容器挂载到 `/var/opt/memos` 的实际宿主机路径，确认该目录含有现存的 SQLite 数据库，再把该路径填入服务的 `volumes`。Compose 可能会为写错的路径创建空目录。原实例使用命名卷时，沿用原挂载和项目名称。
- 把原来的自定义镜像、主机端口及绑定地址直接填入 `image` 和 `ports`，把实际运行 UID、GID 和时区填入 `environment` 中的 `PUID`、`PGID` 和 `TZ`。旧配置使用 `MEMOS_UID` 或 `MEMOS_GID` 覆盖时，填写原部署实际生效的值。
- 新容器沿用原容器的反向代理环境和数据卷。旧版通过环境变量配置的消息收件仍可沿用原变量，也可在网页的「通知」和「网络」保存设置。 [容器环境变量](#可选的容器环境变量) · [收件说明](#telegram-和企业微信收件)

在部署目录拉取镜像并重建。

```sh
docker compose pull
docker compose up -d
```

若固定了镜像标签，拉取前请在 `image` 中填入希望升级到的版本。启动后用原账号登录，检查已有笔记和附件是否可读。想迁移笔记时，可在「设置 → 导出和导入」导出 ZIP。 [数据备份与恢复](.agents/skills/cat-two-product-development/references/product-guide.md#数据备份和升级)

## 构建和发布镜像

在 GitHub Actions 打开 **Cat_two Container**，选择 `main` 和 `dockerhub` 并运行工作流。它使用仓库 Secrets `DOCKERHUB_USERNAME` 和 `DOCKERHUB_TOKEN`，验证后发布 AMD64 和 ARM64 镜像。也可以在推送到 `main` 的最新提交消息加入 `[publish dockerhub]` 触发发布。手动选择 `ghcr` 时，镜像发布到 `ghcr.io/tiger-zero/cat_two`。 [容器工作流](.github/workflows/cat-two-container.yml)

Docker Hub 版本从 `0.01` 起，每次发布递增 `0.01`，例如 `0.09` 的下一版是 `0.10`。每次发布同时提供编号、`latest` 和 `sha-<完整提交 SHA>` 三种标签。要固定版本，直接将 `compose.yaml` 中的 `image` 改为 `docker.io/tiger2896/cat_two:<版本号>`。 [Docker Hub 镜像](https://hub.docker.com/r/tiger2896/cat_two)

也可以在独立检出目录构建本地镜像。先用 pnpm 构建前端，再让 Docker 将静态文件嵌入服务端。

```sh
(cd web && pnpm install --frozen-lockfile && pnpm release)
docker build -f scripts/Dockerfile \
  --build-arg VERSION="$(bash scripts/release_version.sh development-version)" \
  --build-arg COMMIT="$(git rev-parse HEAD)" \
  -t cat-two:local .
```

把 `compose.yaml` 中的 `image` 改为 `cat-two:local`，再执行 `docker compose up -d`。

## 开发文档

按工作内容查阅项目开发规范、产品功能与 API，或桌面 UI 设计与验收。开发记录、优化方案和设计归档可从项目开发 Skill 找到。 [项目开发](.agents/skills/cat-two-development/SKILL.md) · [产品功能](.agents/skills/cat-two-product-development/SKILL.md) · [UI 设计与开发](.agents/skills/cat-two-ui-development/SKILL.md)

## 项目来源与许可

Cat_two 基于 Fumos 和 Memos，保留原项目的 MIT 许可与来源记录。后续开发以本仓库的 `main` 为基础。 [Fumos](https://github.com/Tiger-zero/Fumos) · [Memos](https://github.com/usememos/memos) · [MIT 许可](LICENSE)
