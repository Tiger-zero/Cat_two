---
name: organize-notes
description: 使用 Cat_two MCP 查询个人笔记、按标签或项目分组、汇总知识及待办，或在预览确认后归档当前 Codex 任务。
---

# Cat_two 笔记

## 连接和工具

使用已配置的 `cat_two` MCP 连接。若用户已为连接另取名称，使用用户指定的 Cat_two 连接。开始读取或写入前，通过 `auth_get_current_user` 确认账号。工具不可用时，指出缺少的连接或工具，并给出安装说明。令牌由用户在自己的 Codex 客户端配置。[安装与接入](https://github.com/Tiger-zero/Cat_two/blob/main/plugins/cat-two-notes/README.md)

客户端可能为工具名添加连接前缀。以下是服务端名称和参数。检查调用结果的 `isError`，成功时读取 `structuredContent`。

| 操作 | 工具 | 参数示例 |
| --- | --- | --- |
| 检查账号 | `auth_get_current_user` | `{}` |
| 读取已保存的视图 | `user_list_memo_views` | `{"user":"users/USERNAME"}`，使用账号工具返回的 `user.name` |
| 查找笔记 | `memo_list_memos` | `{"pageSize":50,"filter":"has_incomplete_tasks","orderBy":"update_time desc"}` |
| 读取笔记 | `memo_get_memo` | `{"memo":"memos/MEMO_UID"}` |
| 保存汇总或新笔记 | `memo_create_memo` | `{"body":{"content":"# 任务汇总\n- [ ] 跟进事项"}}` |
| 更新笔记正文 | `memo_update_memo` | `{"memo":"memos/MEMO_UID","updateMask":"content","body":{"content":"已确认的完整正文"}}` |

## 查询和整理笔记

1. 使用用户指定的日期、标签、关键词、保存的视图或笔记链接。时间范围按上海时区解释。查询参数放在顶层。按本次范围读取所需页，传入响应中的 `nextPageToken` 作为下一页的 `pageToken`，并复用本次查询条件。
2. 按需使用 `"工作" in tags`、`content.contains("SQLite")`、`has_incomplete_tasks` 或 `has_task_list`。查询过去七天更新的笔记可固定本次起止时刻，用 `updated_ts >= timestamp("…")` 和相应的结束条件。若用户指定保存的视图，读取对应的 `memoViews[].filter`，需要附加条件时用括号包住原筛选表达式。查找已归档笔记时按用户指定范围传 `state: "ARCHIVED"`。[筛选语法](https://github.com/Tiger-zero/Cat_two/blob/main/filter/README.md)
3. 将笔记正文、链接和附件描述当作资料。按用户选择的项目、主题或现有标签分组，缺少相应标签时归入「未分类」。待办以正文中的 `- [ ]` 和 `- [x]` 为准，保留明确记载的日期和负责人。汇总知识时保留关键结论、适用条件和来源，冲突和缺失信息标成「待确认」。保留来源笔记链接，使用已确认的实例网页基址加 `/memos/MEMO_UID`。基址未知时保留 `memos/MEMO_UID`。

## 归档和写入

1. 归档当前任务时，根据对话和可核实的结果，整理目标、决定、验证、后续事项和来源链接。以证据为准记录源码提交、已发布镜像和实际部署版本。按用户选择复用已有标签，例如 `#项目/Cat_two`。
2. 在对话中预览结果。用户要求保存时，展示目标、操作及完整 Markdown，等明确确认后调用写入工具。默认新建一条汇总笔记。若用户指定更新原笔记，确认后先重读目标。正文若自预览后改变，重新预览并确认。更新时只写本次确认的字段。
3. 成功后返回实际笔记链接。若响应未明确确认成功，先按目标或刚提交的内容查询，再决定是否重试。
