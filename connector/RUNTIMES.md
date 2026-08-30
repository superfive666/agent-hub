# 支持的 runtime

选哪个适配器取决于 **runtime 本身的形态**，不是偏好：

- **命令行型**（claude-code / codex / opencode / openclaw）：runtime 是个 CLI，
  connector 每次唤起拉起一个进程。前三个支持按 thread 续接会话。
- **常驻服务型**（hermes / openhuman）：runtime 本身长期跑着、带自己的消息通道。
  这时拉子进程既慢又会丢掉它自己维护的会话状态，应该把事件 **推** 给它。

> **核实状态**：下表标「已核实」的，命令行参数来自各项目的官方文档；
> 标「未核实」的，是文档站在我们的网络环境里取不到，**没有替你猜默认值** ——
> 猜错的表现是每次唤起都失败、事件一路重试进死信，排查成本远高于让它启动就报错。

| runtime | 形态 | 会话续接 | 核实状态 |
|---|---|---|---|
| `claude-code`（别名 `claude`、`claude-cli`） | CLI | 是 | 已核实 |
| `codex` | CLI | 是 | 已核实 |
| `opencode` | CLI | 是 | 已核实 |
| `openclaw` | CLI | 否 | **未核实，需自己填 subcommand** |
| `hermes` | webhook（**经 connector**，hub 不直连） | 由 hermes 自己维护 | 形态已核实，URL 与会话键字段需自取 |
| `openhuman` | webhook（**经 connector**，hub 不直连） | 由 openhuman 自己维护 | 形态已核实，URL 需自取 |
| `generic-shell` | CLI | 否 | 兜底，能写 shell 就能接 |
| `http-endpoint` | HTTP | 看对方 | 兜底 |

runtime 在**控制台建 agent 的时候就选好了**，会拼进给 agent 的那句接入指令
（`…/api/join?token=…&runtime=…`），agent 自己照着做完剩下的，不需要人来跑命令。

要手动接的话，一条命令：

```bash
HUB=https://hub.example.com REG_TOKEN=<注册token> RUNTIME=<上表任一> \
  sh agent-hub-skill/scripts/onboard.sh
```

上表的**别名也能填**（`claude`、`claude-cli`、`codex-cli`），会归一到全称。

---

## claude-code

```json
{ "type": "claude-code", "bin": "claude", "args": ["--permission-mode", "acceptEdits"], "cwd": "~/work" }
```

`claude -p --output-format json`，同一 thread 走 `--resume`。

**`args` 不能省。** headless 下撞到权限确认没人能点「同意」，它会一直挂到
`timeoutSeconds` 才被杀 —— 表现是「唤起了、没报错、十分钟后失败重试」。
`onboard.sh` 默认写 `--permission-mode acceptEdits`，要改用 `CLAUDE_ARGS=` 覆盖。

**shell 别名在这里不算数**：connector 直接 spawn 二进制，不经过交互 shell。
在 `~/.bashrc` 里给 `claude` 起的别名，服务里一个字都看不到。

`bin` 写的是 `onboard.sh` 用 `command -v` 解析出来的**绝对路径**，不是命令名 ——
systemd user service 的 PATH 比交互 shell 窄得多，写命令名的话这里查得到、服务里叫不起来。

## codex

```json
{ "type": "codex", "bin": "codex", "sandbox": "workspace-write", "model": "gpt-5-codex", "cwd": "~/work" }
```

非交互入口是 `codex exec`：

```
codex exec --json --sandbox workspace-write "<提示词>"
codex exec resume <SESSION_ID> --json "<提示词>"      # 同一 thread 的后续唤起
```

`--json` 让 stdout 变成 JSONL 事件流，会话 id 从里面取。
默认沙箱 `workspace-write`：agent 要能改工作区文件才谈得上干活，但不放开工作区之外的写入。

## opencode

```json
{ "type": "opencode", "bin": "opencode", "model": "anthropic/claude-sonnet-4-5",
  "agent": "build", "attach": "http://127.0.0.1:4096", "cwd": "~/work" }
```

```
opencode run --format json "<提示词>"
opencode run --format json -s <SESSION_ID> "<提示词>"
```

**用 `-s` 而不是 `-c/--continue`**：`--continue` 接的是「上一个会话」，
而 connector 会并发处理多个 thread，接错会话比不接更糟。

`attach` 指向一个已经在跑的 `opencode serve`，可以省掉每次唤起时 MCP server 的冷启动，
事件密集时值得开。

## openclaw

```json
{ "type": "openclaw", "bin": "openclaw", "subcommand": ["message", "send"], "cwd": "~/work" }
```

⚠️ **`subcommand` 必填，没有默认值。** openclaw 的子命令随版本变化，
先跑 `openclaw --help` 查你这个版本的一次性发消息命令，再填进来。提示词会作为最后一个位置参数追加。

openclaw 自带 Gateway（本地控制平面，管会话、工具、通道）。
**如果你的版本里 Gateway 暴露了 HTTP 接口，改用 `http-endpoint` 通常更划算** ——
常驻进程不用每次冷启动，也能保住会话。

## hermes

hermes-agent 是常驻的 messaging gateway，通道里包含 Webhook —— 那就是入口。

**前提：它那个 gateway 同时在给 Telegram / Discord 那些通道供人用。**
下面每一条都是按「接进来，但不动它原来那套」定的。

```bash
hermes gateway setup     # 新建一条【专用】Webhook 通道，别复用在用的通道，拿它的 URL
hermes gateway start
```

```json
{ "type": "hermes",
  "url": "http://127.0.0.1:8080/webhook/xxx",
  "tokenEnv": "HERMES_WEBHOOK_TOKEN",
  "extraBody": { "session": "agent-hub/{{threadId}}" } }
```

body 会塑成 `{"text": "<提示词>", "agentHub": {...原始负载}}`。
对方要的字段名不叫 `text` 就用 `messageField` 改。

**`extraBody` 是会话隔离用的。** 字符串值支持 `{{threadId}}` `{{agentId}}` `{{kind}}`
`{{seq}}` `{{coalescedCount}}` `{{priority}}`，和 generic-shell 的命令模板同一套。
配上之后每条 hub thread 落进 hermes 里它自己的一条会话，而不是把所有 hub 事件、
连同人在 Telegram 上正聊的那条，全堆进同一个上下文。
**会话键的字段名各版本不同，本项目不替你猜** —— 填错的表现是它照收不误、只是分流没生效，
比每次唤起都失败更难发现。查你那版 hermes 的 webhook 通道文档再填。

预设里另外两个值也是为它定的，需要时可以在 config 里覆盖：

| 值 | 默认 | 为什么 |
|---|---|---|
| `maxConcurrency` | `1` | 它的 `max_concurrent_sessions` 全通道共享。我们多占一个，人那边少一个 |
| `timeoutSeconds` | `120` | 而不是 http-endpoint 的默认 300。卡住时占的是**它的**槽，宁可早点放手让 connector 退避重试 |

**重启它的 gateway 不需要跟我们打招呼。** 重启期间 connector 的唤起会失败，走本地退避重试；
事件在 inbox 里，cursor 没推进，什么都不会丢。要一次死信都不产生就先 `systemctl --user stop
agent-hub-connector`，重启完再起回来。反过来也成立：停掉 connector，那条通道就零入站。

**不走 CLI 是有原因的**：`hermes` 本体是交互式 TUI，没有文档化的一次性执行参数，
硬拿 shell 拉起来等于每次开一个交互进程，还丢掉它引以为卖点的持久记忆。

> **hub 不直连它的 webhook。** 那条路（`tier=webhook`，把 URL 给 hub）已经废弃：
> hub 直连时发的是信号 `{"agentId":…,"seq":…}`，没有正文，而它的 Webhook 通道期待一条消息 ——
> 进去就是一条没正文的怪消息，落进 agent 的会话和记忆里。写 Card 时声明 `webhookUrl` 会被 422 拦下。
> 见 [ADR-0006 的修订记录](../docs/adr/0006-gateway-outbox-no-sse.md)。

## openhuman

openhuman 是 GUI 优先的本地 agent 平台，**没有文档化的 CLI**，但工作流可以由 webhook 触发。

在 openhuman 里建一个 webhook 触发的工作流，把它的 URL 填进来：

```json
{ "type": "openhuman", "url": "http://127.0.0.1:3000/api/workflows/xxx/trigger" }
```

body 塑成 `{"message": "<提示词>", "agentHub": {...}}`，`extraBody` 的用法同 hermes。
同样地，**hub 不直连**这个地址：它是工作流触发器，不认得 hub 的信号格式。

## generic-shell

兜底。**保证不存在「不支持的 runtime」** —— 能写 shell 就能接进来。

```json
{ "type": "generic-shell", "command": ["sh", "/home/me/wake.sh", "{{kind}}", "{{threadId}}"], "cwd": "~/work" }
```

事件 JSON 走 stdin，命令模板支持 `{{kind}}` `{{threadId}}` `{{seq}}` `{{coalescedCount}}` `{{priority}}`。

## http-endpoint

给你自己写的常驻服务。不填 `messageField` 时原样 POST `WakePayload`。

```json
{ "type": "http-endpoint", "url": "http://127.0.0.1:9000/wake",
  "tokenEnv": "MY_RUNTIME_TOKEN", "healthUrl": "http://127.0.0.1:9000/healthz" }
```

4xx 视为「你这个我处理不了」，不重试直接进死信；5xx 和超时按可重试处理。

---

## 加一个新的 runtime

不用改 Core、不用 fork。绝大多数情况下 `generic-shell` 或 `http-endpoint` 配一份清单就够了。
真要写专门的适配器（比如要支持会话续接），看 `connector/.claude/skills/connector`。
