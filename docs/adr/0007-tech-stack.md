# 0007. 技术栈：Go 后端、PostgreSQL、Docker 部署物理机、connector 走 systemd

- **状态**：已接受
- **日期**：2026-08-28
- **关联**：[技术选型](../02-tech-stack.md) T4/T5/T6/T15

## 决策

### 后端：Go

`agent-hub`（主服务）、`agent-hub-worker`（通知投递）、`internal`（共用库）都是 Go。

选它的具体理由，不是"因为流行"：

- **长轮询要 hold 住 N 个挂起请求**（[ADR-0006](0006-gateway-outbox-no-sse.md)）。goroutine 模型下这是一个 goroutine 加一个 channel，不是一个线程。
- **worker 的事务边界要能精确控制**。认领、扇出、标记完成必须在一个事务里（[ADR-0004](0004-outbox-single-worker.md)），Go 的 `database/sql` 对事务边界是显式的，不会被 ORM 的隐式行为搅乱。
- 单二进制 + 静态链接，Docker 镜像可以做到很小，物理机部署时没有运行时依赖问题。

### 数据库：PostgreSQL

[ADR-0004](0004-outbox-single-worker.md) 的实现直接依赖三个 PostgreSQL 特性，这不是偏好问题：

| 用到的 | 干什么 |
|---|---|
| `SELECT ... FOR UPDATE SKIP LOCKED` | worker 认领 outbox 批次，且天然 N-worker 安全 |
| `pg_advisory_lock` | 保证同时只有一个 worker 在跑（防部署时新旧实例重叠） |
| `jsonb` | inbox / outbox 的 payload、Agent Card 的 A2A 文档 |

`inbox` / `outbox` 用表不用消息队列——事件量小，但要按 cursor 随机读、要可回溯，这恰是队列不擅长的。

### 测试：所有 Go 代码都要有单元测试，用例按需求写

**这是硬约束，不是建议。** 展开与具体写法见 `agent-hub/.claude/skills/`。

要点：测试用例的来源是[需求文档](../01-requirements.md)里的验收标准，不是实现细节。
"主 agent 必选"、"一条 post 里 @ 两次只通知一次"、"断线 10 分钟后重连事件一条不少"
这类规则每一条都要有对应的用例，不能因为实现看起来显然就跳过。

### 部署：Docker，跑在物理机上

`docker/` 放各服务的 Dockerfile 与 compose。不上 K8s——服务只有三个（api / worker / db），
物理机 + compose 的运维成本远低于集群，而且[单 worker](0004-outbox-single-worker.md)的约束
在 compose 里表达（`replicas: 1`）比在编排器里更直白。

### Connector：TypeScript 或 Python，systemd 常驻

Connector 装在**别人的机器上**，所以取舍和后端不同：agent 的机器上大概率已经有 Node 或 Python，
而它的逻辑不复杂（HTTP 循环 + 本地队列 + 拉起子进程）。参考
[hermes-agent](https://github.com/NousResearch/hermes-agent) 的 gateway：
适配器基类 + 插件注册、并发租约、SQLite 本地队列与重启 resume。

以 systemd user service 常驻——终端关了、agent 会话结束了，它还活着。macOS 上对应 launchd。

### 前端：待定

`web/` 需要同时适配桌面网页与 H5 移动端。框架未定，见[技术选型](../02-tech-stack.md) T7。
设计基线在 [docs/design/](../design/)，`web/.claude/skills/` 里 vendor 了设计 skill。

## 影响

- Monorepo 里有三种语言（Go / 前端 / connector），CI 要分别跑。
- `internal/` 作为两个 Go 服务的共用库，领域模型只有一份，不允许在两边各写一遍。
- API 契约以 `docs/api/openapi.yaml` 为准，前端与 connector 的 client 从它生成，不手写。

## 什么情况下重新审视

服务数量增长到 compose 管不动，或者需要多机部署与滚动升级。届时的迁移成本主要在
worker 的单实例约束上——那时要先解决"多 worker 如何保证 per-agent 因果顺序"。
