# Cat_two 笔记插件

在本地 Codex 连接 Cat_two 后，可以读写笔记。可选的 Cat_two 笔记插件提供按标签、项目和视图整理笔记、汇总待办或知识、归档当前任务的工作流。

## 连接 NAS

1. 在运行 Codex 的电脑上登录 Cat_two，打开「设置 → MCP 和 API」。复制 MCP 服务地址，确认末尾是 `/mcp`，并使用 NAS 对外映射的端口。有 HTTPS 地址时优先使用 HTTPS。NAS 仅在局域网可达时，使用能访问该地址的电脑开启 Codex 本地会话。
2. 在同一页新增「MCP 连接」，名称可以填「我的 Codex」。复制创建时显示的 **Bearer 请求头值**。专用令牌只在创建时显示。已有 REST 集成继续使用个人访问令牌（PAT）。旧版使用 PAT 的 MCP 客户端需要更新为专用令牌。
3. 在 Codex 桌面端打开「设置 → MCP servers」，添加自定义服务，选择 **Streamable HTTP**，名称填 `cat_two`，URL 填复制的 `/mcp` 地址。如果界面提供自定义请求头，名称填 `Authorization`，值填复制的 `Bearer <MCP 连接令牌>`。如果界面只提供 URL，按下面的环境变量示例设置 Bearer 认证。保存后重启或重连 MCP 服务。 [Codex MCP 设置](https://developers.openai.com/codex/extend/mcp)
4. Codex 第一次带令牌连接时，Cat_two 会将连接标为待审批，并向客户端返回 `403`。回到 Cat_two 的「设置 → MCP 和 API」，找到该连接，选择批准 24 小时、永久批准或拒绝。24 小时从批准时开始计算。
5. 获批后用原令牌重连 Codex，必要时重启客户端并开启新对话。想停止访问时可以在同一页撤销连接。连接已过期时重新创建令牌并审批。

### 用环境变量传入令牌

在 Windows 搜索「编辑账户的环境变量」，创建用户变量 `CAT_TWO_MCP_TOKEN`。变量值只填写令牌本身，省略开头的 `Bearer `。使用 Bash 时，可以运行 `read -rs CAT_TWO_MCP_TOKEN` 隐藏输入，再运行 `export CAT_TWO_MCP_TOKEN`，从同一终端启动 Codex。

打开本机 Codex 的 `~/.codex/config.toml`，Windows 默认路径是 `%USERPROFILE%\.codex\config.toml`。把下例的 URL 换成实例地址。已有 `[mcp_servers.cat_two]` 时更新原条目，并清理旧的认证设置。保存后完全退出并重新打开 Codex。

```toml
[mcp_servers.cat_two]
url = "https://cat-two.example.com/mcp"
bearer_token_env_var = "CAT_TWO_MCP_TOKEN"
enabled_tools = [
  "auth_get_current_user",
  "user_list_memo_views",
  "memo_list_memos",
  "memo_get_memo",
  "memo_create_memo",
  "memo_update_memo",
]
default_tools_approval_mode = "writes"
```

Cat_two 每次收到 MCP 请求都会检查授权。专用令牌仅能调用账户、视图和笔记的六个工具，读取或写入依然受笔记的所属账号权限约束。用插件生成笔记时，工作流会在写入前预览并等待确认。

### 排查连接

如果连接失败，先在运行 Codex 的设备上打开实例的 `/healthz` 核对网络和端口，再检查「设置 → MCP 和 API」里的连接状态。仍显示等待客户端连接时，核对 `/mcp` 地址和 Bearer 请求头。使用环境变量时，确认运行 Codex 的进程加载了 `CAT_TWO_MCP_TOKEN`。已批准且未到期时，可在 Codex 中重新连接或重启客户端。 [Cat_two MCP 接口](../../server/mcp/README.md#connecting-a-client)

## 安装笔记插件

如果使用 Codex 桌面端，将本地 Cat_two 仓库更新到包含本插件的版本，再将其作为本地项目打开。在插件目录选择 **Cat_two** 来源，安装 **Cat_two 笔记**。若目录里暂时没有该来源，重启桌面端后再查看。

也可以在本地 Cat_two 仓库根目录用支持插件的 Codex CLI 安装。

```sh
codex plugin marketplace add .
codex plugin add cat-two-notes@cat-two
```

已安装旧版时，更新本地仓库后可再次运行 `codex plugin add cat-two-notes@cat-two` 安装新版。插件也可用于同一台电脑上的其他 Codex 项目。 [Codex 插件文档](https://developers.openai.com/codex/plugins/build)

## 使用示例

在新对话中选择 **Cat_two 笔记** 或用 `$cat-two-notes:organize-notes` 调用工作流。

- “用 Cat_two 笔记检查连接，并显示当前账号和最近三条笔记。”
- “用 Cat_two 笔记按已有标签汇总过去七天的未完成待办，保留原笔记链接。”
- “用 Cat_two 笔记把这次开发整理成项目归档，先给我看草稿。”
