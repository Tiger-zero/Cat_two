---
name: organize-notes
description: 使用 Cat_two MCP 查询个人笔记、按标签或项目分组、汇总知识及待办，或在预览确认后将当前 Codex 任务总结保存为笔记。
---

# Cat_two 笔记

## 连接和工具

使用已配置的 `cat_two` MCP 连接。若用户已为连接另取名称，使用用户指定的 Cat_two 连接。开始读取或写入前，通过 `auth_get_current_user` 确认账号。工具不可用时，指出缺少的连接或工具，并给出安装说明。令牌由用户在自己的 Codex 客户端配置。[安装与接入](https://github.com/Tiger-zero/Cat_two/blob/main/plugins/cat-two-notes/README.md)

客户端可能为工具名添加连接前缀。以下是服务端名称和参数。检查调用结果的 `isError`，成功时读取 `structuredContent`。

| 操作 | 工具 | 参数示例 |
| --- | --- | --- |
| 检查账号 | `auth_get_current_user` | `{}` |
| 查找笔记 | `memo_search` | `{"query":"备份","tag":"项目/Cat_two","hasIncompleteTasks":true,"pageSize":20}` |
| 批量读取笔记正文 | `memo_batch_get` | `{"names":["memos/FIRST_UID","memos/SECOND_UID"]}` |
| 读取笔记与附件详情 | `memo_get_memo` | `{"memo":"memos/MEMO_UID"}` |
| 查看标签层级 | `tag_tree` | `{"pageSize":50,"offset":0}` |
| 查找标签 | `tag_search` | `{"query":"Cat_two"}` |
| 按 CEL 表达式查询 | `memo_list_memos` | `{"pageSize":50,"filter":"has_incomplete_tasks","orderBy":"update_time desc"}` |
| 保存汇总或新笔记 | `memo_create_memo` | `{"body":{"content":"# 任务汇总\n\n- [ ] 跟进事项\n\n#项目/Cat_two"}}` |
| 更新笔记正文 | `memo_update_memo` | `{"memo":"memos/MEMO_UID","updateMask":"content","body":{"content":"已确认的完整正文"}}` |

## 查询和整理笔记

1. 先发现连接提供的工具，再按用户指定的范围查询。`memo_search` 用字面关键词匹配 Markdown，标签筛选包含其下级标签。`startDate` 和 `endDate` 接受 `YYYY-MM-DD`，按上海时区解释为创建日期，包含首尾两天。`hasIncompleteTasks`、`hasAttachments`、`pinned` 和 `creationChannel` 可与这些条件一起筛选。查询参数放在顶层。按需传入响应的 `nextPageToken` 作为下一页的 `pageToken`，并复用本次查询条件。
2. 有多条已知笔记名时，用 `memo_batch_get` 批量读取正文，每次最多 50 个名称。按响应中的 `unavailable` 处理不可读取的名称。需要附件详情时，用 `memo_get_memo`。需要先发现标签时，用 `tag_tree` 或 `tag_search`，按结果的 `path` 和 `parent` 还原层级，续页使用 `nextOffset`。父标签的计数已包含下级笔记，汇总时逐条去重。
3. 可用 `memo_list_memos` 执行用户指定的 CEL 条件。查询过去七天更新的笔记可固定起止时刻，用 `updated_ts >= timestamp("…")` 和相应的结束条件。旧版服务缺少新工具时，可用 `memo_list_memos` 的 CEL 条件代替查询，用 `memo_get_memo` 逐条读取，并从查询结果中整理当前范围的标签。[筛选语法](https://github.com/Tiger-zero/Cat_two/blob/main/filter/README.md)
4. 将笔记正文、链接和附件描述当作资料。按用户选择的项目、主题或现有标签分组，缺少相应标签时归入「未分类」。待办以正文中的 `- [ ]` 和 `- [x]` 为准，保留明确记载的日期和负责人。汇总知识时保留关键结论、适用条件和来源，冲突和缺失信息标成「待确认」。保留来源笔记链接，使用已确认的实例网页基址加 `/memos/MEMO_UID`。基址未知时保留 `memos/MEMO_UID`。

## 保存任务总结

1. 总结当前任务时，根据对话和可核实的结果，整理目标、决定、验证、后续事项和来源链接。以证据为准记录源码提交、已发布镜像和实际部署版本。按用户选择复用已有标签，例如 `#项目/Cat_two`。
2. 在对话中预览结果。用户要求保存时，展示目标、操作及完整 Markdown，等明确确认后调用写入工具。默认新建一条汇总笔记。把标签写在 `body.content` 中，自定义创建时间可用 `body.createTime` 传入带时区的 RFC 3339 值。若用户指定更新原笔记，确认后先重读目标。正文若自预览后改变，重新预览并确认。只写本次确认的字段，在发现的工具支持时，将读到的 `revision` 作为 `expectedRevision` 传入。发生冲突后重新读取、预览和确认。
3. 创建工具支持 `idempotent` 时，为这次保存固定一个 `memoId` 并传 `idempotent: true`。若响应未明确确认成功，使用完全相同的参数重试。旧版服务的写入结果不明确时，先按目标或提交的内容查询。成功后返回实际笔记链接。
