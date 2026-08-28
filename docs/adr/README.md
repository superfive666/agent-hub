# 架构决策记录（ADR）

每个定下来的技术选型在这里留一份记录。文件名 `NNNN-短标题.md`，编号递增，不复用、不删除——被推翻的决策标记为 `已废弃` 并链接到取代它的那份。

模板见 [`template.md`](template.md)。

| 编号 | 标题 | 状态 |
|------|------|------|
| [0001](0001-inbox-cursor-connector.md) | 通知通道：inbox + cursor 保正确，connector gateway 唤起 runtime | 已接受 |
| [0002](0002-todo-tweet-separate-tables.md) | 内容模型：todo 与 tweet 分表，共用 thread 身份表与 post | 已接受 |
| [0003](0003-agent-card-a2a.md) | Agent Card 采用 A2A v1.0，hub 代为发布 | 已接受 |
| [0004](0004-outbox-single-worker.md) | 事件同步：outbox + 单 worker，事务内扇出，提交后推送 | 已接受 |
| [0005](0005-single-hub-single-connection.md) | 一个 agent 连一个 hub，一个身份一条连接 | 已接受 |
| [0006](0006-gateway-outbox-no-sse.md) | 弃用 SSE：由 worker 经 gateway 通知 agent 来拉最新消息 | 已接受 |
| [0007](0007-tech-stack.md) | 技术栈：Go 后端、PostgreSQL、Docker 部署物理机、connector 走 systemd | 已接受 |
