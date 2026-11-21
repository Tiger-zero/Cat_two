---
name: cat-two-mcp
description: >-
  将 Codex 或其他 AI 客户端接入 Docker 或 NAS 上的 Cat_two MCP 服务。
  适用于读取和分组笔记、汇总待办和知识、归档当前 Codex 任务，以及预览并确认后保存或更新笔记。
---

# Cat_two MCP

使用 Cat_two 容器自带的 `/mcp`。在「设置 → MCP 和 API」创建专用 MCP 连接令牌，选择从创建起 7 天（默认）、14 天、1 个日历年或永久。复制 MCP 地址和完整的 Bearer 请求头值，令牌创建后即可使用。个人访问令牌（PAT）用于 REST。 [Cat_two 接入说明](../../../server/mcp/README.md#connecting-a-client)

## 连接实例

- Codex 使用原生 Streamable HTTP 连接。Bearer 令牌环境变量留空，在 `Authorization` 请求头中粘贴 `Bearer <MCP 连接令牌>`。保存后按客户端提示重启。笔记插件可以另行安装。 [Codex 接入配置](../../../plugins/cat-two-notes/README.md#连接-nas)
- 其他 AI 客户端用自己的 Streamable HTTP 配置连接同一服务。客户端与 Cat_two 在同一主机时可用 `http://127.0.0.1:5230/mcp`。NAS 部署使用客户端实际可访问的地址及主机端口，端口由 `CAT_TWO_PORT` 配置。令牌留在该客户端的受保护设置中。 [普通部署](../../../compose.yaml) · [NAS 部署](../../../compose.nas.yaml)
- 连接问题先核对网络、端口、实例的 `/healthz`、客户端的完整 Bearer 请求头和网页上的令牌状态。设置显示首次连接的客户端 IP 和令牌到期时间。令牌到期或连接删除后，后续请求需要使用新令牌。旧版等待连接或待审批的令牌也需重新创建。浏览器客户端若发送 `Origin`，需使用与代理 `Host` 匹配的来源或实例配置允许的来源。 [探活路由](../../../server/server.go) · [来源检查](../../../server/mcp/origin.go)

## 使用 MCP 整理笔记

按 [Cat_two 笔记工作流](../../../plugins/cat-two-notes/skills/organize-notes/SKILL.md) 发现工具、核对当前账号、整理笔记和归档任务。该文件同样用于已安装的 Codex 插件。笔记写入前展示完整草稿并取得用户确认，工具名和参数以部署实例返回的列表为准。 [工具定义](../../../server/mcp/catalog.go) · [REST 字段](../../../proto/gen/openapi.yaml)

工具列表支持时，用 `memo_search` 按关键词、标签或上海时区的创建日期筛选，用 `memo_batch_get` 读取多条已知笔记，用 `tag_tree` 和 `tag_search` 发现标签。较早的镜像可用 `memo_list_memos` 配合 `memo_get_memo`。 [MCP 工具说明](../../../server/mcp/README.md#tool-surface)

同步总结时，将正文作为 Markdown 写入 `content`。标题、表格、任务列表、提示块等语法可查项目的格式清单。 [支持的内容格式](../cat-two-development/SKILL.md#支持的内容格式)
