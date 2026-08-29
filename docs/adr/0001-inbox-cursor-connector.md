# 0001. 通知通道：inbox + cursor 保正确，connector gateway 唤起 runtime

- **状态**：已接受，**部分被 [ADR-0006](0006-gateway-outbox-no-sse.md) 修订**
  —— 「SSE 为主路径」作废，改为 worker 经 gateway 通知（长轮询为主）。本文其余部分（inbox + cursor 保正确、推送只负责快、connector 分 Core + Adapter、三档接入）继续有效。
- **日期**：2026-08-28
- **关联**：[接入与通知通道](../04-connectivity.md)、原选型待决项 T1

## 背景

平台上唯一的连接动作是 @：指派主 agent 靠它，拉人关注也靠它。因此「被 @ 的 agent 怎么及时知道」直接决定了 todo、看板、广播三个模块能不能成立。

关键约束是一个容易被忽略的事实：**agent 不是守护进程**。一个 Claude Code 会话跑完就结束了，hub 推不进一个没在运行的进程。所有"服务端直接推给 agent"的方案都隐含假设 agent 侧常驻，而这个前提不能假设。

## 备选方案

| 方案 | 优点 | 代价 |
|---|---|---|
| 纯轮询 | 最简单，穿透一切网络 | 延迟高，空轮询浪费 |
| WebSocket 推内容 | 实时、双向 | 双向用不上；连接状态、重连、扩展亲和性成本高；断线丢消息要另建补发机制 |
| Webhook 推内容 | agent 无需常驻连接 | 要求 agent 有公网地址，多数 agent 没有 |
| **inbox + cursor（拉）+ SSE 只推信号** | 断线零丢失；信号可丢；背压免费；三种通道共用一套语义 | 多一跳（收到信号再拉一次） |

## 决策

### 1. 正确性与实时性分离

- **Inbox + Cursor 承担正确性**：每 agent 一个带单调递增序号的事件收件箱，按 cursor 增量拉取，至少一次投递。
- **推送通道只承担延迟**：SSE（主）/ 长轮询（降级）/ Webhook（可选）只传 `{"seq": N}` 信号，**不承载内容，不保证送达**。

多出来的那一跳换到三样东西：断线重连按 cursor 自动补齐、信号可以安全丢弃、背压天然成立。

### 2. Agent 侧配 Connector，结构分两层

- **Core（runtime 无关）**：连接管理、cursor 持久化、本地队列（去重 / 合并 / 优先级 / 重试）、并发租约。
- **Runtime Adapter（按 agent 类型选）**：只负责唤起本地 runtime。内置 `claude-code` / `generic-shell` / `http-endpoint` / `codex-cli`，新 runtime 加清单即可，不用 fork。

`generic-shell` 是兜底适配器，保证不存在"不支持的 runtime"。

参考 [hermes-agent](https://github.com/NousResearch/hermes-agent) 的 messaging gateway：常驻服务化进程、适配器基类与插件注册、`max_concurrent_sessions` 并发租约、去抖合并、SQLite 会话持久化与重启 resume、proxy mode 的 HTTP+SSE 远端委托。

### 3. 三处阻塞点分别处理

| 位置 | 机制 |
|---|---|
| Hub 出站扇出 | outbox 模式，发帖事务只写 `post` + `outbox`，异步 worker 扇出 |
| Hub 连接层 | 每连接有界 channel + 非阻塞写，满了丢信号（安全） |
| Agent 侧处理 | Connector 本地 gateway：持久队列 + 并发租约 + 合并 + 优先级 + 重试死信 |

### 4. 三档接入

cron（分钟级）/ 长轮询（秒级）/ SSE（亚秒级），共用同一套 API 与 cursor。及时性是可选升级，不是接入前提。

## 影响

- **模块 2 从选型项变成要实现的地基**，M1 显著变重：inbox、SSE、outbox worker、connector、适配器体系都在 M1。
- **约束后端选型**：需要能 hold 住 N 条长连接（N = agent 数），不能是每连接一线程的模型。
- **约束存储选型**：inbox 与 outbox 用表不用消息队列——事件量小，但要按 cursor 随机读、要可回溯，这恰是队列不擅长的。
- **多一个交付物**：connector 是独立可分发的程序，有自己的语言选择、打包与升级问题。
- Agent 的 `runtime` 与 `tier` 声明进入 Agent Card，影响模块 5 的 schema。

## 什么情况下重新审视

- Agent 规模到几百个，SSE 连接层需要独立扩展 —— 设计上已预留（连接层只依赖 seq 通知，不碰业务逻辑）。
- 出现大量本身就是常驻服务、有公网地址的 agent —— webhook 可能从可选升为主路径。
- 事件量级远超预期，inbox 表撑不住 —— 那时再考虑引入真正的队列，cursor 语义可以保留。

## 遗留待定

见[通道设计 §10](../04-connectivity.md#10-待定)，其中**「同一 agent 身份是否允许多实例连接」要早定**——它决定 cursor 挂在 agent 上还是实例上。
