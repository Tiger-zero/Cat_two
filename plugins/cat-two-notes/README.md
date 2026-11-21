# Cat_two 笔记插件

在 Codex 中按标签、项目和视图整理 Cat_two 笔记，汇总知识或待办，也可以把当前任务写成 Markdown 归档草稿。插件使用 Cat_two 容器已有的 MCP 服务。安装后需要在自己的 Codex 宿主机上配置实例地址和令牌。

## 安装

如果使用 Codex 桌面端，将本地 Cat_two 仓库更新到包含本插件的版本，再将其作为本地项目打开。在插件目录选择 **Cat_two** 来源，安装 **Cat_two 笔记**。若目录里暂时没有该来源，重启桌面端后再查看。

也可以在本地 Cat_two 仓库根目录用支持插件的 Codex CLI 安装。

```sh
codex plugin marketplace add .
codex plugin add cat-two-notes@cat-two
```

插件也可用于同一台电脑上的其他 Codex 项目。 [Codex 插件文档](https://developers.openai.com/codex/plugins/build)

## 连接 NAS

1. 登录 Cat_two，打开「设置 → MCP 和 API」并创建个人访问令牌。Codex 所在设备需要能访问 NAS。服务地址使用浏览器打开 Cat_two 的地址加 `/mcp`，例如 `http://NAS_IP:5230/mcp`。如果已经配置 HTTPS 域名，使用该域名。
2. 将令牌设为运行 Codex 的进程可以读取的 `CAT_TWO_PAT` 环境变量。在 Windows 可从「编辑账户的环境变量」添加该用户变量，再完全退出并重新打开 Codex。在 Bash 中也可以用 `read -rs CAT_TWO_PAT` 隐藏输入，再运行 `export CAT_TWO_PAT`，从同一终端启动 Codex。
3. 打开本机 Codex 的 `~/.codex/config.toml`，在 Windows 上即 `%USERPROFILE%\.codex\config.toml`。添加下列配置，并换成自己的服务地址。已配置 `mcp_servers.cat_two` 时，修改已有项。重新启动 Codex 并开启新对话。 [Codex MCP 配置](https://developers.openai.com/codex/extend/mcp)

```toml
[mcp_servers.cat_two]
url = "http://NAS_IP:5230/mcp"
bearer_token_env_var = "CAT_TWO_PAT"
enabled_tools = ["auth_get_current_user", "user_list_memo_views", "memo_list_memos", "memo_get_memo"]
default_tools_approval_mode = "writes"
```

按需在 `enabled_tools` 中加入 `memo_create_memo` 和 `memo_update_memo`，允许保存新笔记和更新正文。插件在写入前预览完整内容，等待确认。令牌本身仍具有 Cat_two 账号的权限，可在「MCP 和 API」撤销。

## 使用示例

在新对话中选择 **Cat_two 笔记** 或用 `$cat-two-notes:organize-notes` 调用工作流。

- “用 Cat_two 笔记按已有标签汇总过去七天的未完成待办，保留原笔记链接。”
- “用 Cat_two 笔记把这次开发整理成项目归档，先给我看草稿。”

如果连接失败，先用运行 Codex 的设备打开实例的 `/healthz` 核对网络和端口，然后确认 `CAT_TWO_PAT` 已加载且有效。NAS 仅在局域网可达时，使用能访问该局域网的本地 Codex 客户端。 [Cat_two MCP 接口](../../server/mcp/README.md#connecting-a-client)
