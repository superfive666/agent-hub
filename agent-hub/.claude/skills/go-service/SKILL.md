---
name: go-service
description: 在 agent-hub / agent-hub-worker / internal 里写 Go 代码时用。覆盖分层约定、单元测试要求（按需求写用例，不按实现写）、事务边界、错误语义、幂等、outbox worker 的实现纪律、Docker 构建与物理机部署。任何新增 Go 包、改动事务或事件路径、写测试之前都要先读。
---

# agent-hub 的 Go 服务开发

先读根 [CLAUDE.md](../../../CLAUDE.md) 的三条设计前提和 [docs/adr/](../../../docs/adr/)。
**ADR 是有约束力的**，代码里绕过某条 ADR 之前先去改 ADR。

## 1. 分层

```
cmd/<binary>/main.go     只做装配：读配置、连库、起服务。不放业务逻辑
internal/<domain>/       领域逻辑，纯函数优先，不认识 HTTP 也不认识 SQL
internal/store/          SQL。每个方法接收 context 和可选的 *sql.Tx
internal/api/            HTTP handler。只做解析、鉴权、调用领域层、序列化
internal/event/          inbox / outbox 事件类型与优先级定义
```

仓库根的 `internal/` 是**两个 Go 服务共用**的库，领域模型只有一份。
`agent-hub` 和 `agent-hub-worker` 都不许把领域模型再抄一遍。

给 agent 的 API 和给 admin 前端的 API 走两套路由（`/api/agent/*` 与 `/api/admin/*`），
鉴权模型和数据形状差异很大，但共用领域层。

## 2. 测试：硬约束

**所有 Go 代码必须有单元测试。用例的来源是[需求文档](../../../docs/01-requirements.md)里的验收标准，不是实现。**

写测试之前先去需求文档里找这个功能对应的验收标准，一条一条转成用例。
"实现看起来很显然所以不用测"是最常见的借口，也是最常出事的地方——
下面这些规则每一条都简单到"显然"，每一条都值一个用例：

| 需求里的规则 | 必须有的用例 |
|---|---|
| 主 agent 必选且唯一 | 不给 `primary_agent_id` 创建 todo 必须失败，且错误可读 |
| @ 只产生关注者 | 被 @ 的 agent 拿到 `todo.mentioned`，工作队列里**没有**这条 |
| 主 agent 同时被 @ 不重复入队 | 只产生一条事件，取优先级最高的类型 |
| 一条 post 里 @ 两次只通知一次 | `mention` 表主键挡掉重复，只发一条 |
| 作者不收自己的通知 | 扇出结果里不含 `actor_agent_id` |
| 断线 10 分钟后重连事件一条不少 | 按 cursor 拉取，前后事件序号连续无缺口 |
| 至少一次投递 | 同一事件重复拉取时，agent 侧按事件 id 能去重 |
| 吊销凭证立即失效 | 吊销后挂起的长轮询请求终止，后续调用被拒 |
| 没有预置管理员则启动失败 | 配置缺失时构造函数返回错误，不是默认放行 |
| cron 档 agent 不被判离线 | 按档位取不同的在线判定窗口 |

### 怎么写

- **表驱动**，用例名说清楚场景，不是 `TestFoo1`：

  ```go
  tests := []struct {
      name    string
      give    CreateTodoInput
      wantErr error
  }{
      {name: "没有主 agent 时拒绝创建", give: ..., wantErr: ErrPrimaryAgentRequired},
      {name: "主 agent 同时被 @ 时只入队一次", give: ..., wantErr: nil},
  }
  ```

- **数据库相关的逻辑用真 PostgreSQL 测**，不要用 mock 假装。
  `SKIP LOCKED`、advisory lock、事务隔离级别的行为 mock 不出来，而这些恰恰是 outbox 方案的地基。
  CI 里用 service container，本地用 `docker/compose.test.yml`。
- **并发行为要真并发地测**：两个 worker 同时认领 outbox、两个 agent 同时申领同一条开放任务、
  新连接顶替旧连接。用 `t.Parallel()` 加 `sync.WaitGroup`，不要靠 sleep 碰运气。
- **不测私有实现细节**。测的是"给定这个输入，对外可观察的行为是什么"。
  重构不该让测试大面积飘红——如果会，说明测试贴着实现写了。
- 断言用 `got`/`want` 命名，失败信息要能直接看出差在哪，不要只 `t.Fail()`。

### 门槛

- 新增或修改的包，`go test ./...` 必须过。
- 领域层（`internal/<domain>/`）覆盖率不低于 85%；`store` 与 `api` 层不设硬指标，
  但每条需求规则都要有对应用例。
- **覆盖率是下限不是目标**。为了凑数写的空测试比没有测试更糟——它让人以为测过了。

## 3. 事务边界

这是最容易写错、错了最难查的地方。三条规则：

**① 发帖与 outbox 必须同事务**

```go
tx, _ := db.BeginTx(ctx, nil)
defer tx.Rollback()
store.InsertPost(ctx, tx, post)
store.InsertOutbox(ctx, tx, event)   // 同一个 tx
tx.Commit()
```

这保证「帖子发成功了，通知就一定会到」。分开写就会出现帖子在、通知丢——这类 bug 几乎查不出来。

**② worker 的认领、扇出、标记完成必须在同一事务**

```
SELECT ... FROM outbox_event WHERE status='pending' AND next_attempt_at <= now()
  ORDER BY id LIMIT 100 FOR UPDATE SKIP LOCKED
  → 算收件人集合 → 分配 seq → INSERT inbox_event ON CONFLICT DO NOTHING
  → UPDATE outbox_event SET status='done'
COMMIT
```

崩了就回滚，outbox 行回到 pending 重跑。所以写进 inbox 这一段是 **exactly-once**。

**③ 通知必须在 `COMMIT` 之后发**

```go
if err := tx.Commit(); err != nil { return err }
gateway.Notify(agentID, seq)   // 只能在这里
```

在事务里就通知的话，agent 收到通知立刻来拉，事务还没提交——它什么都拉不到，
要等下次轮询才发现。**这个坑在低负载下几乎不出现**，所以只能靠纪律，不能靠调试发现。
写一个用例专门盯它：通知回调里立即查 inbox，必须查得到。

## 4. 错误语义

返回给 agent 的错误要能被 agent **自己读懂并据此决策**：能不能重试、多久后重试、是不是永久失败。

```go
type APIError struct {
    Code       string `json:"code"`        // "rate_limited" / "token_used" / "not_primary_agent"
    Message    string `json:"message"`     // 人和 agent 都能看懂的一句话
    Retryable  bool   `json:"retryable"`
    RetryAfter int    `json:"retryAfter,omitempty"` // 秒
}
```

不要用裸 HTTP 状态码打发 agent。`429` 不带 `retryAfter` 等于让它自己猜，猜出来的行为就是重试风暴。

## 5. 幂等

Agent 重试是常态，不是异常。所有写接口接受 `Idempotency-Key`，同 key 同结果。
实现上落一张 `idempotency_key(key, agent_id, response, created_at)`，
命中就直接回放上次的响应，不重复执行。

## 6. Worker 的实现纪律

- **单实例**：启动时取 `pg_advisory_lock`，取不到就退出（不是等待）。主要是防部署时新旧实例重叠那几秒。
- **代码写成 N-worker 安全**（`SKIP LOCKED` + `ON CONFLICT DO NOTHING` + 事务边界），
  部署只跑一个。现在不多付复杂度，将来要扩不用改代码。
- **顺序**：outbox 按 `id` 升序处理，per-agent 的 seq 因此就是因果顺序。
  任何"为了快"打乱这个顺序的优化都要先改 ADR-0004。
- **重试**：`attempts++`，指数退避写 `next_attempt_at`；超过上限置 `dead`，**并且要让 admin 看得见**。
- **`outbox_lag` 指标必须暴露**：`now() - min(occurred_at) WHERE status='pending'`。
  这是唯一能发现 worker 静默死亡的地方。

## 7. 配置与启动

- 配置从环境变量读，见根 `.env.example`。
- **没有预置管理员时必须启动失败**，返回明确错误。不能默认放行——
  那会悄悄跑起一个谁都能进的实例。这条要有用例。
- 启动时校验数据库 schema 版本，不匹配就拒绝启动，不要自动迁移。

## 8. Docker 与部署

- 多阶段构建：`golang:x-alpine` 编译 → `gcr.io/distroless/static` 运行，静态链接单二进制。
- `docker/` 下每个服务一个 Dockerfile，compose 编排 api / worker / postgres。
- **worker 在 compose 里写死 `replicas: 1`**，并在文件里注释清楚为什么。
- 部署在物理机，不上 K8s——三个服务用编排器管的成本远高于收益。

## 9. 提交前

```bash
gofmt -l .            # 必须为空
go vet ./...
golangci-lint run
go test ./... -race   # -race 不是可选项，这个项目里到处是并发
```
