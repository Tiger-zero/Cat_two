# Cat_two 笔记插件

在 Codex 中按标签、项目和视图整理 Cat_two 笔记，汇总知识或待办，也可以把当前任务写成 Markdown 归档草稿。插件使用 Cat_two 容器已有的 MCP 服务。安装后需要在自己的 Codex 宿主机上配置实例地址和令牌。

## 安装

如果使用 Codex 桌面端，将本地 Cat_two 仓库更新到包含本插件的版本，再将其作为本地项目打开。在插件目录选择 **Cat_two** 来源，安装 **Cat_two 笔记**。若目录里暂时没有该来源，重启桌面端后再查看。

也可以在本地 Cat_two 仓库根目录用支持插件的 Codex CLI 安装。

```sh
codex plugin marketplace add .
codex plugin add cat-two-notes@cat-two
```

已安装旧版时，更新本地仓库后可再次运行 `codex plugin add cat-two-notes@cat-two` 安装新版。插件也可用于同一台电脑上的其他 Codex 项目。 [Codex 插件文档](https://developers.openai.com/codex/plugins/build)

## 连接 NAS

1. 在运行 Codex 的电脑上打开 NAS 的 Cat_two 网页，登录后进入「设置 → MCP 和 API」。复制页面上的「MCP 服务地址」。也可以在网页地址后加 `/mcp`，例如 `http://NAS_IP:5230/mcp`。端口使用 NAS 对外映射的端口。
2. 在同一页的「访问令牌」点击「创建令牌」。第一次创建时按钮显示「创建您的第一个令牌」。描述可填 `Codex`，点击「创建」，然后复制对话框里显示的令牌。
3. 将令牌设为运行 Codex 的进程可以读取的 `CAT_TWO_PAT` 环境变量。在 Windows 可搜索「编辑账户的环境变量」，在用户变量里新建 `CAT_TWO_PAT`，将令牌粘贴到变量值。在 Bash 中可以用 `read -rs CAT_TWO_PAT` 隐藏输入，再运行 `export CAT_TWO_PAT`，从同一终端启动 Codex。
4. 打开本机 Codex 的 `~/.codex/config.toml`，在 Windows 上即 `%USERPROFILE%\.codex\config.toml`。使用下列配置并换成自己的服务地址。已经配置过 `[mcp_servers.cat_two]` 时，更新已有的工具列表。保存后完全退出再打开 Codex，开启本地对话。 [Codex MCP 配置](https://developers.openai.com/codex/extend/mcp)

```toml
[mcp_servers.cat_two]
url = "http://NAS_IP:5230/mcp"
bearer_token_env_var = "CAT_TWO_PAT"
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

这份配置默认开放读取、创建和更新笔记。插件在写入前预览完整内容，等待确认。配置中的工具列表约束 Codex 可调用的工具，令牌本身按对应 Cat_two 账号的权限访问，可在「MCP 和 API」撤销。

接入现有 Cat_two Docker 服务时，在运行 Codex 的电脑上完成以上配置即可。NAS 地址仅在局域网可达时，在能访问该地址的电脑上使用 Codex 的本地会话。

## 使用示例

在新对话中选择 **Cat_two 笔记** 或用 `$cat-two-notes:organize-notes` 调用工作流。

- “用 Cat_two 笔记检查连接，并显示当前账号和最近三条笔记。”
- “用 Cat_two 笔记按已有标签汇总过去七天的未完成待办，保留原笔记链接。”
- “用 Cat_two 笔记把这次开发整理成项目归档，先给我看草稿。”

如果连接失败，先用运行 Codex 的设备打开实例的 `/healthz` 核对网络和端口，然后确认 `CAT_TWO_PAT` 已加载且有效。 [Cat_two MCP 接口](../../server/mcp/README.md#connecting-a-client)
