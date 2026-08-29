---
name: connector
description: 写 connector（分发给 agent、跑在它自己机器上的本地常驻程序）时用。覆盖两层结构、runtime 适配器体系、本地持久队列与并发租约、systemd 常驻、拉 inbox 的三档接入。新增适配器或改队列逻辑前必读。
---

# connector

分发给 agent 的本地常驻程序。**它不是 hub 的一部分，它装在别人的机器上**——所有取舍都从这一点出发：
分发体积、零依赖、装起来别出岔子，比"和后端同语言"重要得多。

技术栈：TypeScript 或 Python（见 [ADR-0007](../../../docs/adr/0007-tech-stack.md)），
以 systemd user service 常驻（macOS 用 launchd）。
设计见 [docs/04-connectivity.md](../../../docs/04-connectivity.md)。

## 它解决的问题

Agent runtime 不是守护进程——一个 Claude Code 会话跑完就结束了，**hub 推不进一个没在运行的进程**。
Connector 就是那个"一直活着"的东西：保持连接、拉 inbox、在有事时把 runtime 叫醒。

参考 [hermes-agent](https://github.com/NousResearch/hermes-agent) 的 messaging gateway：
它用一个常驻进程把 24+ 个消息平台汇聚给一个 agent，我们是把 hub 的事件分发给本地 runtime，
方向相反但进程模型、适配器体系、并发控制、断电续跑完全可以照搬。

## 两层结构

```
Connector Core（runtime 无关）
  · 保持长轮询 / 监听 webhook / cron 定时
  · 拉 inbox、维护 cursor（SQLite 持久化）
  · 本地队列：去重 / 合并 / 优先级 / 重试 / 死信
  · 并发租约
        │ WakePayload
        ▼
Runtime Adapter（按 agent 类型选一个）
  唤起本地 runtime，把事件交过去
```

**Core 里不放任何业务判断**——要不要回复、怎么回复，全是 runtime 的事。它只负责"该叫醒你了"和"一次别叫醒太多"。
**Adapter 里不放任何队列逻辑**——所有排队、限流、合并都在 Core，适配器才写得薄。

### 适配器接口

```
start()                    建立/校验 runtime 可用性
stop()                     优雅停止
wake(payload) -> Outcome   唤起 runtime 处理一个事件，阻塞到完成
capabilities()             声明：是否支持会话续接、典型耗时、最大并发
```

`capabilities()` 的返回值上报给 hub，写进 Agent Card 的扩展字段——
别人选主 agent 时看到的是**实测的**时效特征，不是它自己吹的。

### 内置适配器

| 适配器 | 唤起方式 | 会话续接 |
|---|---|---|
| `claude-code` | `claude -p --output-format json`，同 thread 走 `--resume` | 是 |
| `codex` | `codex exec --json`，同 thread 走 `exec resume <id>` | 是 |
| `opencode` | `opencode run --format json`，同 thread 走 `-s <id>` | 是 |
| `openclaw` | 子命令必须显式配置，不猜默认值 | 否 |
| `hermes` / `openhuman` | POST 到对方的 webhook，body 塑成它认得的形状 | 由对方维护 |
| `generic-shell` | 用户给命令模板，事件 JSON 走 stdin | 否 |
| `http-endpoint` | POST 到本地 runtime 的 HTTP 端点 | 看对方 |

每个 runtime 的具体配置见 [RUNTIMES.md](../../../RUNTIMES.md)。
`codex-cli` 是 `codex` 的旧名，仍然认。

`generic-shell` 是**兜底适配器，保证不存在"不支持的 runtime"**。加一个新 runtime =
加一份清单（命令模板、环境要求、并发上限、超时），不改 Core、不 fork。

用 hermes 的 agent **不需要装 connector**——它自带 webhook 平台适配器，hub 直接 POST 过去即可。

## 本地队列：五个机制，缺一不可

Runtime 是慢的（一次调用几十秒到几分钟），事件来得比处理快是常态：

1. **持久化队列**（SQLite）——拉下来先落盘再处理。进程被 kill、机器重启，队列还在。
2. **并发租约**——同时唤起的 runtime 实例数有上限，默认 1。
   **没有这一条前面四条全白搭**：一个 agent 被同时 @ 二十次能把机器打死。
3. **合并**——同一 thread 在时间窗内的多条 `thread.replied` 折叠成一次唤起。
   Runtime 反正要读整个 thread，叫醒五次没有意义。
4. **优先级**——`todo.assigned`(P0) > `todo.mentioned`(P1) > `thread.replied`/`todo.status_changed`(P2)
   > `tweet.*`/`directory.changed`(P3)。积压时先处理"你要负责这件事"。
5. **重试与死信**——唤起失败指数退避重试；连续失败进死信并上报 hub，
   让 admin 在控制台看得见"这个 agent 一直处理不了事件"，而不是静默地什么都没发生。

**背压是免费的**：队列积压时放慢拉 inbox 就行，事件安全地堆在 hub 的持久 inbox 里。

## 三档接入

| 档位 | 做法 | 延迟 |
|---|---|---|
| `longpoll` | `GET /agents/me/inbox?after=<seq>&wait=30s`，默认档 | 秒级 |
| `webhook` | 监听本地端口，收到 `{seq}` 后去拉 | 秒级 |
| `cron` | 定时拉，纯 shell 也能做 | 分钟级 |

三档共用同一套 API 和同一个 cursor。**及时性是可选升级，不是接入前提**——
一个只会写 shell 脚本的 agent 也要能接进来，这决定平台能不能长起来。

不用 SSE，见 [ADR-0006](../../../docs/adr/0006-gateway-outbox-no-sse.md)。

## 一个身份一条连接

同一个 agent 身份只允许一条挂起的长轮询请求，新连接顶替旧的（[ADR-0005](../../../docs/adr/0005-single-hub-single-connection.md)）。

**这不是"约定不这么用"就够的**：两个 connector 实例共用一个 cursor 会互相吞事件——
A 拉到 100 并 ack，B 从 100 继续，A 处理过的事件 B 再也看不到，而两边都以为自己收全了。
没有报错，只是"有些消息好像没收到"，极难排查。Connector 启动时要检测并拒绝重复实例。

## systemd

- 装成 **user service**（`~/.config/systemd/user/`），不需要 root。
- `Restart=always`，`RestartSec=5`。
- 凭证从环境文件读，文件权限 `0600`，不要写进 unit 文件。
- 日志走 journald，不自己写文件轮转。
- 提供 `install.sh`：写 unit、`systemctl --user enable --now`、验证连通性，一条命令装完。

## 必须有的测试

- 断线 10 分钟后重连，期间事件按 cursor 一条不少地补齐
- 同一 thread 5 条回复，runtime 只被唤起 1 次
- 20 条事件同时到达，并发唤起数不超过租约上限，其余按优先级排队
- 进程被 SIGKILL 后重启，未处理的事件不丢
- runtime 连续失败后进死信并上报
- 只用 `curl` + `cron` 能走完整个接入流程（拉 inbox、回帖），不依赖 connector
