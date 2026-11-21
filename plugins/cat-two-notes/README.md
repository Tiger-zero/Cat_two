# Cat_two 笔记插件

在本地 Codex 连接 Cat_two 后，可以读写笔记。可选的 Cat_two 笔记插件提供按标签、项目和视图整理笔记、汇总待办或知识、归档当前任务的工作流。

## 连接 NAS

1. 在运行 Codex 的电脑上登录 Cat_two，打开「设置 → MCP 和 API」，创建 MCP 连接令牌。有效期默认是从创建起 24 小时，也可选永久。令牌创建后立即生效。
2. 复制 MCP 服务地址和完整的 **Bearer 请求头值**。地址以 `/mcp` 结尾，使用 NAS 映射的端口，有 HTTPS 地址时优先使用 HTTPS。令牌只在创建时显示。
3. 在本地 Codex 桌面端打开「设置 → MCP servers」，添加自定义服务并选择 **Streamable HTTP**。名称可填 `cat_two`，URL 粘贴复制的 MCP 地址。将 **Bearer token env var** 留空，在 HTTP 请求头中添加 `Authorization`，值粘贴完整的 `Bearer <MCP 连接令牌>`，保留开头的 `Bearer` 和空格。
4. 保存设置，按客户端提示重启或开启新对话。NAS 仅在局域网可达时，使用能访问该地址的电脑开启 Codex 本地会话。 [Codex MCP 设置](https://developers.openai.com/codex/extend/mcp)

如果 Codex 版本的界面只提供 URL，可以在本机的 `~/.codex/config.toml` 配置相同的请求头。Windows 默认路径为 `%USERPROFILE%\.codex\config.toml`。替换下面的地址和占位令牌，已有 `[mcp_servers.cat_two]` 时更新原有配置，并移除旧的 `bearer_token_env_var`。

```toml
[mcp_servers.cat_two]
url = "https://cat-two.example.com/mcp"
http_headers = { Authorization = "Bearer <MCP 连接令牌>" }
default_tools_approval_mode = "writes"
```

Cat_two 每次收到 MCP 请求都会检查令牌。专用令牌仅适用于 MCP，可调用账户、视图和笔记的六个工具，笔记读写受所属账号权限约束。已有 REST 集成继续使用个人访问令牌（PAT）。用笔记插件写入时，工作流会在保存前预览并等待确认。

### 升级与撤销

旧版已批准的 MCP 令牌继续沿用原有效期。旧版等待连接或待审批的令牌需要重新创建。已拒绝、已撤销或已过期的令牌均无法连接。可在「设置 → MCP 和 API」撤销有效令牌，再按需创建新令牌。

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
- “用 Cat_two 笔记按已有标签汇总过去七天的未完成待办，保留原笔记链接。”
- “用 Cat_two 笔记把这次开发整理成项目归档，先给我看草稿。”
