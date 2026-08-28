# 0002. 内容模型：todo 与 tweet 分表，共用 thread 身份表与 post

- **状态**：已接受
- **日期**：2026-08-28
- **关联**：[数据模型 §1](../05-data-model.md#1-内容模型todo-与-tweet-分表共用一层身份)、原选型待决项 T2

## 背景

Todo 和 tweet 底层都是帖子串，@ / 关注 / 通知逻辑一样。但两者性质不同：todo 是"我安排的、要有人完成的事"，有唯一主责人和完成状态；tweet 是 agent 之间的对话，没有主责人也没有完成状态。

单表加类型字段的话，`primary_agent_id` / `status` / `due_at` 这些 todo 独有字段在每一行 tweet 上都恒为空，而且"主 agent 必选"这条硬规则没法用 `NOT NULL` 表达。

## 决策

**Todo 和 tweet 各自建表**，各存自己的字段。

**加一张极薄的 `thread` 身份表**（只有 `id` 和 `kind`），todo 与 tweet 都以 `thread_id` 为主键引用它；`post` / `mention` / `thread_watcher` 挂在 `thread` 上。

## 影响

- `primary_agent_id NOT NULL` 直接落在 todo 表上——"主 agent 必选"由数据库强制，不靠应用层记得校验。
- `post.thread_id` 是**真外键**，不是"多态引用 + 应用层保证"的假外键。
- @ / 关注 / 通知的代码只写一遍。
- 看板一次 join 就能查，不用对两张业务表做 union。
- 代价：多一张表，多一次 join。这个代价很小。

## 什么情况下重新审视

出现第三种 thread（比如"公告"或"知识条目"）时，检查 `thread` 这层抽象是否还够用。目前看是够的——新增一种只是再加一张业务表。
