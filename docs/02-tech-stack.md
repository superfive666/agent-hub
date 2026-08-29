# 技术选型

> **状态：已定案。** 技术栈已经定完，结论在下面，每项都有对应 [ADR](adr/)。剩下的只有前端框架一项。

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
| Connector | TypeScript 或 Python，systemd 常驻，参考 hermes gateway | [0007](adr/0007-tech-stack.md) |
| 仓库布局 | monorepo，见根 [CLAUDE.md](../CLAUDE.md) | — |
| CI | **暂缓**。计划开源后再配 GitHub Actions；在那之前靠本地 `go test ./... -race` 与提交前检查 | — |

## 还没定

### T7 · 前端框架 ⚠️ 唯一剩下的阻塞项

`web/` 要同时适配**桌面网页**与 **H5 移动端**。需要考虑：

- thread 视图与看板日期导航的交互复杂度
- 长轮询在浏览器侧的处理（admin 控制台也要实时看到 thread 更新）
- 移动端的导航形态（侧栏在窄屏怎么收）

设计基线在 [design/](design/)，`web/.claude/skills/` 里 vendor 了 impeccable 与 taste-skill。

### 局部待定

这些不阻塞开工，散在各文档的「待定」小节里：

- Connector 具体用 TypeScript 还是 Python
- inbox 事件保留期与清理策略
- 一个 connector 进程能否带多个不同 agent 身份
- `skills[]` 是否需要 hub 给一份 tag 词表以便做能力匹配
