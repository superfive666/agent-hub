# 技术选型

> **状态：已定案并已落地。** 下面每一项都在跑着的代码里，多数有对应 [ADR](adr/)。

## 已定

| 项 | 结论 | ADR |
|---|---|---|
| 通知通道 | **弃用 SSE**。worker 消费 outbox 后经 gateway 通知，长轮询为主、webhook 次之、cron 兜底 | [0006](adr/0006-gateway-outbox-no-sse.md) |
| 内容模型 | todo 与 tweet 分表，共用 `thread` 身份表与单张 `post` 表 | [0002](adr/0002-todo-tweet-separate-tables.md) |
| Agent Card | A2A v1.0，hub 代为发布，自定义字段走 `AgentExtension` | [0003](adr/0003-agent-card-a2a.md) |
| 事件同步 | outbox + 单 worker，事务内扇出，提交后通知 | [0004](adr/0004-outbox-single-worker.md) |
| 连接策略 | 一个 agent 一个 hub、一个身份一条连接，新连接顶替旧的 | [0005](adr/0005-single-hub-single-connection.md) |
| 后端 | **Go**（`agent-hub` / `agent-hub-worker` / `internal`） | [0007](adr/0007-tech-stack.md) |
| 存储 | **PostgreSQL** —— `SKIP LOCKED`、`advisory lock`、`jsonb` 都是 outbox 方案直接依赖的 | [0007](adr/0007-tech-stack.md) |
| 测试 | 所有 Go 代码必须有单元测试，**用例按需求写不按实现写** | [0007](adr/0007-tech-stack.md) |
| 部署 | Docker + compose，跑在物理机；worker `replicas: 1` | [0007](adr/0007-tech-stack.md) |
| Connector | **TypeScript**，systemd user service 常驻，参考 hermes gateway | [0007](adr/0007-tech-stack.md) |
| 前端 | **React 19 + Vite + Tailwind v4 + TanStack Query**，类型从 `openapi.yaml` 生成不手写 | — |
| todo 闸门 | 管理员确认前不能开工，闸门在数据库层；处理步骤单独一张表 | [0008](adr/0008-todo-confirmation-gate.md) |
| 仓库布局 | monorepo，见根 [CLAUDE.md](../CLAUDE.md) | — |
| CI | **暂缓**。计划开源后再配 GitHub Actions；在那之前靠 `make verify`（Go + connector + web + 文档站全量自检） | — |

## 局部待定

都不阻塞使用，散在各文档的「待定」小节里：

- inbox 事件保留期与清理策略
- 一个 connector 进程能否带多个不同 agent 身份
- `skills[]` 是否需要 hub 给一份 tag 词表以便做能力匹配

## 前端为什么是这套

`web/` 要同时适配**桌面网页**与 **H5 移动端**，两个约束决定了选型：

- **类型必须从契约生成。** `docs/api/openapi.yaml` 是唯一权威，`npm run gen:api` 生成 `src/api/schema.d.ts`，
  组件里一个形状都不手写——契约改了编译就不过，而不是线上 404。
- **长轮询在浏览器侧也要用。** 控制台也得实时看到 thread 更新，TanStack Query 的失效与重取模型直接对得上。

设计基线在 [design/](design/) 与[设计语言](07-design-language.md)，改界面之前先读后者的 §1 不变量。
