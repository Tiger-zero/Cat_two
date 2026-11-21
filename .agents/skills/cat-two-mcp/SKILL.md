---
name: cat-two-mcp
description: >-
  将 Codex 或其他 AI 客户端接入 Docker 或 NAS 上的 Cat_two MCP 服务。
  适用于读取和分组笔记、汇总待办和知识、归档当前 Codex 任务，以及预览并确认后保存或更新笔记。
---

# Cat_two MCP

使用 Cat_two 容器自带的 `/mcp`。在「设置 → MCP 和 API」创建专用 MCP 连接令牌，客户端选择 Streamable HTTP，并通过 `Authorization: Bearer <MCP 连接令牌>` 认证。第一次携带令牌连接会进入待审批，获批后用同一令牌重连。个人访问令牌（PAT）用于 REST。 [Cat_two 接入说明](../../../server/mcp/README.md#connecting-a-client)

## 连接实例

- Codex 使用原生 Streamable HTTP 连接。客户端支持时设置 Bearer 请求头，也可在本机配置 `mcp_servers.cat_two` 的 `bearer_token_env_var`。笔记插件可以另行安装。 [Codex 接入配置](../../../plugins/cat-two-notes/README.md#连接-nas)
- 其他 AI 客户端用自己的 Streamable HTTP 配置连接同一服务。客户端与 Cat_two 在同一主机时可用 `http://127.0.0.1:5230/mcp`。NAS 部署使用客户端实际可访问的地址及主机端口，端口由 `CAT_TWO_PORT` 配置。令牌留在该客户端的受保护设置中。 [普通部署](../../../compose.yaml) · [NAS 部署](../../../compose.nas.yaml)
- 连接问题先核对客户端网络、端口和实例的 `/healthz`。收到 `403` 时请用户在网页检查连接状态。待审批可选择批准 24 小时、永久批准或拒绝，已批准的连接可撤销。列表中没有申请时，再核对令牌和代理转发的请求头。浏览器客户端若发送 `Origin`，需使用与代理 `Host` 匹配的来源或实例配置允许的来源。 [探活路由](../../../server/server.go) · [来源检查](../../../server/mcp/origin.go)

## 使用 MCP 整理笔记

按 [Cat_two 笔记工作流](../../../plugins/cat-two-notes/skills/organize-notes/SKILL.md) 发现工具、核对当前账号、整理笔记和归档任务。该文件同样用于已安装的 Codex 插件。笔记写入前展示完整草稿并取得用户确认，工具名和参数以部署实例返回的列表为准。 [工具定义](../../../server/mcp/catalog.go) · [REST 字段](../../../proto/gen/openapi.yaml)
