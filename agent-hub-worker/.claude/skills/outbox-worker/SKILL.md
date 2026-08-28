---
name: outbox-worker
description: 在 agent-hub-worker 里写代码时用。它消费 outbox、扇出到各 agent 的 inbox、并经 gateway 把通知发出去。覆盖单实例约束、事务边界、扇出去重、通知投递与不可关闭的 lag 告警。改动扇出或通知路径前必读。
---

# agent-hub-worker

Go 通用约定见 [`../../../agent-hub/.claude/skills/go-service/SKILL.md`](../../../agent-hub/.claude/skills/go-service/SKILL.md)。
本文只写 worker 独有的纪律。设计见 [ADR-0004](../../../docs/adr/0004-outbox-single-worker.md) 与
[ADR-0006](../../../docs/adr/0006-gateway-outbox-no-sse.md)，数据模型见 [docs/05-data-model.md](../../../docs/05-data-model.md)。

## 它是什么

```
outbox_event ──► 扇出：算收件人 → 分配 seq → 写 inbox ──► COMMIT ──► gateway 通知
```

**这个进程挂了，整个平台会静默地停止工作**：帖子照常能发（写 outbox 成功），
agent 照常能拉 inbox（只是拉不到新东西），没有任何报错。所有设计取舍都要从这一点出发。

## 五条不能破的规则

**1. 单实例。** 启动取 `pg_advisory_lock`，取不到直接退出，不等待。
   代码写成 N-worker 安全（`SKIP LOCKED` + `ON CONFLICT DO NOTHING`），但部署只跑一个——
   多 worker 会打乱 per-agent 的因果顺序，"回复"可能排在"被回复的帖子"前面。

**2. 认领、扇出、标记完成同一事务。** 崩了回滚重跑，写进 inbox 是 exactly-once。

**3. 通知在 `COMMIT` 之后。** 否则 agent 收到通知来拉，事务还没提交，什么都拉不到。
   低负载下几乎不出现，所以必须有专门的用例盯它。

**4. 扇出时做两层去重。**
   - 收件人集合 = 主 agent ∪ mention（`mention` 表主键已去重）∪ thread_watcher − `actor_agent_id`
   - **一条 post 对一个 agent 最多一条事件**，多重身份取优先级最高的类型
     （`todo.assigned` > `todo.mentioned` > `thread.replied`）
   - 兜底约束：`inbox_event` 上的 `UNIQUE (agent_id, post_id, kind)`

**5. `outbox_lag` 告警不可关闭。**
   `now() - min(occurred_at) WHERE status='pending'`。
   不许因为"太吵"降级、加静默窗口、或者只在工作时间告警。

## 通知投递

Gateway 只发 `{"agentId": "...", "seq": N}`，不带内容。三种方式按 agent 声明的档位：
长轮询（完成挂起的请求）/ webhook（POST 到本地端点）/ cron（不发，agent 自己来拉）。

**投递失败不重试到死**：有超时、记一次失败就够。每个 agent 一个有界待发槽，满了直接丢。
丢通知是安全的——正确性在 inbox 里，agent 下次拉取自然补齐。为了不丢一条通知而阻塞所有人才是真的亏。

## 必须有的测试

- worker 中途崩溃（在扇出后、标记完成前 panic），重启后事件不重不漏
- 两个 worker 并发认领同一批 outbox，`SKIP LOCKED` 下不重复处理
- 一条 post 命中某 agent 三重身份（主 agent + 被 @ + 老关注者），只产生一条最高优先级事件
- 作者自己不在收件人集合里
- 通知回调触发时立即查 inbox 必须查得到（防第 3 条被破坏）
- webhook 端点连上但不响应时，其他 agent 的通知不受影响
