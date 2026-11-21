# Cat_two 笔记插件

在本地 Codex 连接 Cat_two 后，可以读写笔记。可选的 Cat_two 笔记插件提供按标签、项目和视图整理笔记、汇总待办或知识、归档当前任务的工作流。

## 连接 NAS

1. 在运行 Codex 的电脑上登录 Cat_two，打开「设置 → MCP 和 API」，点击「创建 MCP 连接」。专用令牌以 `cat_two_mcp_` 开头，可选 7 天（默认）、14 天、1 年或永久有效。1 年按创建时间加一个日历年计算。令牌创建后即可连接。
2. 复制 MCP 服务地址和弹窗中的 **Authorization 值**，其中包含 `Bearer ` 前缀。地址以 `/mcp` 结尾，使用 NAS 映射的端口，有 HTTPS 地址时优先使用 HTTPS。令牌只在创建时显示。
3. 在本地 Codex 桌面端打开「设置 → MCP servers」，添加自定义服务并选择 **Streamable HTTP**。名称可填 `cat_two`，URL 粘贴复制的 MCP 地址。将 **Bearer token env var** 留空，在 HTTP 请求头中添加 `Authorization`，值粘贴完整的 `Bearer <MCP 连接令牌>`，保留开头的 `Bearer` 和空格。
4. 保存设置，按客户端提示重启或开启新对话。NAS 仅在局域网可达时，使用能访问该地址的电脑开启 Codex 本地会话。 [Codex MCP 设置](https://developers.openai.com/codex/extend/mcp)

如果 Codex 版本的界面只提供 URL，可以在本机的 `~/.codex/config.toml` 配置相同的请求头。Windows 默认路径为 `%USERPROFILE%\.codex\config.toml`。替换下面的地址和占位令牌，已有 `[mcp_servers.cat_two]` 时更新原有配置，并移除旧的 `bearer_token_env_var`。

```toml
[mcp_servers.cat_two]
url = "https://cat-two.example.com/mcp"
http_headers = { Authorization = "Bearer <MCP 连接令牌>" }
default_tools_approval_mode = "writes"
```

Cat_two 每次收到 MCP 请求都会检查令牌。专用令牌适用于 MCP，账户、视图、笔记和标签的读取受所属账号权限约束。「REST API 访问令牌」区签发的个人访问令牌（PAT）以 `memos_pat_` 开头，用于 REST API。用笔记插件写入时，工作流会在保存前预览并等待确认。

### 查看和删除连接

「设置 → MCP 和 API」显示首次连接的客户端 IP，以及连接所用令牌的到期时间。「已连接」表示令牌已使用过。连接随令牌到期而失效。在列表中点击「删除令牌」并确认后，客户端的下一次 MCP 请求会被拒绝，即使此前已经连接。

升级前签发的有效令牌沿用原到期时间。旧版等待连接或待审批的令牌需要重新创建。已拒绝、已撤销或已过期的令牌仍处于不可用状态。

### 排查连接

如果连接失败，先在运行 Codex 的设备上打开实例的 `/healthz` 核对网络和端口，再检查 `/mcp` 地址、完整的 `Authorization` 请求头及 Cat_two 的令牌状态。确认令牌有效后，可在 Codex 中重新连接或按提示重启。 [Cat_two MCP 接口](../../server/mcp/README.md#connecting-a-client)

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
- “查找和 Cat_two 有关的标签，按层级列出标签与笔记数。”
- “用 Cat_two 笔记按已有标签汇总过去七天的未完成待办，保留原笔记链接。”
- “用 Cat_two 笔记把这次开发整理成项目归档，先给我看草稿。”

工作流用 `memo_search` 按字面关键词、标签与创建日期查询，用 `memo_batch_get` 读取多条已知的笔记，并用 `tag_tree` 和 `tag_search` 查找标签。创建日期按上海时区计算。连接较早的镜像时，工作流可以用已有的列表和逐条读取工具完成查询。 [工具参数和范围](../../server/mcp/README.md#tool-surface)
