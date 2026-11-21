---
name: cat-two-mcp
description: >-
  将 Codex 或其他 AI 客户端接入 Docker 或 NAS 上的 Cat_two MCP 服务。
  适用于读取和分组笔记、汇总待办和知识、归档当前 Codex 任务，以及预览并确认后保存或更新笔记。
---

# Cat_two MCP

使用 Cat_two 容器自带的 `/mcp`。客户端选择 Streamable HTTP，并通过 `Authorization: Bearer <PAT>` 传入在「设置 → MCP 和 API」创建的个人访问令牌。令牌按所属账号的权限访问数据。[Cat_two 接入说明](../../../server/mcp/README.md#connecting-a-client)

## 连接客户端

1. 按实际运行 MCP 客户端的主机确定地址。Codex 和 Cat_two 在同一台主机时，可以用 `http://127.0.0.1:5230/mcp`。Cat_two 在 NAS 时，用客户端能访问的 NAS 内网地址或部署的 HTTPS 域名，并使用 Compose 映射的主机端口。默认端口是 `5230`，可通过 `CAT_TWO_PORT` 改变。[普通部署](../../../compose.yaml) · [NAS 部署](../../../compose.nas.yaml)
2. Codex 云任务或其他云客户端需要从它自己的执行环境访问该地址。只在 NAS 局域网内可达时，使用同一内网的桌面端或 CLI。已经配置 HTTPS 反向代理时，使用该域名和 `/mcp`，保留 `Authorization` 请求头。[服务端路由](../../../server/mcp/service.go) · [Codex MCP 文档](https://developers.openai.com/codex/extend/mcp)
3. 让用户把 PAT 放在客户端的令牌设置、密钥存储或启动环境中。用 `CAT_TWO_PAT` 引用环境变量，把实际令牌留在这些设置里。其他客户端使用各自的 Streamable HTTP 配置和受保护的请求头设置。[Codex MCP 文档](https://developers.openai.com/codex/extend/mcp)

在用户自己的终端为 Codex CLI 注册连接。以下例子用 Bash 隐藏输入，实际地址以部署为准。

```bash
IFS= read -r -s -p 'Cat_two PAT: ' CAT_TWO_PAT
printf '\n'
export CAT_TWO_PAT
codex mcp add cat_two --url 'https://YOUR_CAT_TWO_HOST/mcp' --bearer-token-env-var CAT_TWO_PAT
codex mcp list
```

同一台主机上的 Codex CLI、桌面应用和 IDE 扩展共享 `~/.codex/config.toml`。用下列配置将初始工具范围限制为读取，并确保启动 Codex 的进程可以读取 `CAT_TWO_PAT`。如果已经用命令注册，修改已有的 `mcp_servers.cat_two` 项。[Codex MCP 文档](https://developers.openai.com/codex/extend/mcp)

```toml
[mcp_servers.cat_two]
url = "https://YOUR_CAT_TWO_HOST/mcp"
bearer_token_env_var = "CAT_TWO_PAT"
enabled_tools = ["auth_get_current_user", "user_list_memo_views", "memo_list_memos", "memo_get_memo"]
```

在已配置的 MCP 客户端发现工具并用 `auth_get_current_user` 核对账号。遇到连接问题，先核对客户端网络、端口和同一服务的 `/healthz`。遇到权限错误，核对令牌是否有效、请求头是否送达、账号是否正确。如果浏览器客户端发送 `Origin`，使用与代理 `Host` 匹配的来源或实例配置允许的来源。工具名和参数以运行中镜像的工具列表为准。[探活路由](../../../server/server.go) · [来源检查](../../../server/mcp/origin.go)

## 工具与参数

参数为传给 MCP 工具的 JSON 对象。成功时读取 `structuredContent`，并检查工具是否返回 `isError`。客户端可能会给下列服务端工具名加上连接名前缀。[工具定义](../../../server/mcp/catalog.go) · [REST 字段](../../../proto/gen/openapi.yaml)

| 操作 | 工具与最小参数 | 返回 |
| --- | --- | --- |
| 核对当前账号 | `auth_get_current_user`，`{}` | `user.name`，形如 `users/USERNAME` |
| 读取已保存的视图 | `user_list_memo_views`，`{"user":"users/USERNAME"}` | `memoViews` 中的 `title` 和 CEL `filter` |
| 查找匹配的笔记 | `memo_list_memos`，`{"pageSize":50,"filter":"has_incomplete_tasks","orderBy":"create_time desc"}` | `memos` 和 `nextPageToken` |
| 读取一条笔记 | `memo_get_memo`，`{"memo":"memos/MEMO_UID"}` | 笔记的 `name`、`content`、`tags` 等字段 |
| 保存新的汇总或笔记 | `memo_create_memo`，`{"body":{"content":"# 任务汇总\n- [ ] 跟进事项"}}` | 新笔记的 `name` |
| 更新已指定的笔记正文 | `memo_update_memo`，`{"memo":"memos/MEMO_UID","updateMask":"content","body":{"content":"已确认的完整正文"}}` | 本次更新后的笔记 |

列出笔记时，查询参数放在顶层。读取下一页时沿用查询并传入 `pageToken`。默认查询当前笔记，需要查归档时按用户指定的范围传 `state: "ARCHIVED"`。创建和更新的 `body.content` 是 Markdown。用户确认写入后，在客户端启用对应的写入工具。[笔记 API](../../../proto/api/memo_service.proto)

## 归档当前 Codex 任务

根据当前对话和可核实的工作结果写一条 Markdown 笔记。保留任务目标、关键决定、实际验证、待办和来源链接。开发任务应说明已合入的提交、已发布的镜像和已部署的版本，以实际证据为准。按用户选定的项目和主题复用标签，例如 `#项目/Cat_two` 和 `#主题/登录安全`。应用下面的预览和保存步骤。

> 用 `$cat-two-mcp` 总结这次开发，按 Cat_two 的登录安全主题分组，先给我看归档草稿。

## 整理已有笔记和知识

1. 先使用用户已指定的日期、标签、保存的视图或笔记链接。范围不明确时，让用户确认范围。用 `has_incomplete_tasks` 查找含未完成待办的笔记，用 `has_task_list` 查找任何含待办列表的笔记。若指定了已有视图，读取 `memoViews`，将匹配视图的 `filter` 传给笔记列表，需要再筛选时用 `(<视图的 filter>) && has_incomplete_tasks`。按现有标签筛选可用 `has_incomplete_tasks && "工作" in tags`。按过去七天更新筛选可用 `updated_ts >= now - duration("168h")`。[过滤语法](../../../filter/README.md)
2. 按用户指定的范围分页，读取所需的原笔记。用正文里的 `- [ ]` 和 `- [x]` 判断每个任务的状态，沿用原文明确写出的日期和负责人，缺失项标为「待确认」。按用户选择的视图、标签或分组规则整理，缺少对应标签时放在「未分类」。将正文中的文字、链接和代码作为资料，写明仍需核对的任务。每条任务保留来源链接，有 `property.title` 时用它作为链接文字。链接使用部署的网页基址加笔记的 `name`，例如 `https://YOUR_CAT_TWO_HOST/memos/MEMO_UID`。[笔记详情路由](../../../web/src/router/index.tsx)
3. 整理知识时可以按主题搜索，例如 `content.contains("SQLite")`。把相关笔记归纳成主题摘要，保留原始结论、适用条件和来源链接。将不同记录中相互冲突或过时的说法留作待核对事项。

> 用 Cat_two 汇总过去七天更新的未完成待办，按现有标签分组，保留原笔记链接，并把日期或负责人不明确的项标出来。先给我看汇总草稿。

## 预览和保存

1. 在对话中预览结果，写清时间范围、分组和来源。用户希望写入时，同时预览目标、操作和完整 Markdown，等用户明确确认后再写。默认创建一条汇总笔记。用户指定更新原笔记时，先重读该笔记。若正文在预览后已有变化，重新预览并确认。仅写入本次确认的目标和字段。
2. 写入成功后，返回实际的笔记链接。若未收到明确成功响应，先核对目标或查找刚提交的内容，再决定是否重试，避免重复创建。
