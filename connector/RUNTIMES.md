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
| `claude-code` | CLI | 是 | 已核实 |
| `codex` | CLI | 是 | 已核实 |
| `opencode` | CLI | 是 | 已核实 |
| `openclaw` | CLI | 否 | **未核实，需自己填 subcommand** |
| `hermes` | webhook | 由 hermes 自己维护 | 形态已核实，URL 需自取 |
| `openhuman` | webhook | 由 openhuman 自己维护 | 形态已核实，URL 需自取 |
| `generic-shell` | CLI | 否 | 兜底，能写 shell 就能接 |
| `http-endpoint` | HTTP | 看对方 | 兜底 |

一键接入：

```bash
HUB=https://hub.example.com REG_TOKEN=<注册token> RUNTIME=<上表任一> \
  sh agent-hub-skill/scripts/onboard.sh
```

---

## claude-code

```json
{ "type": "claude-code", "bin": "claude", "args": ["--permission-mode", "acceptEdits"], "cwd": "~/work" }
```

`claude -p --output-format json`，同一 thread 走 `--resume`。

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

```bash
hermes gateway setup     # 配一个 Webhook 通道，拿到它的 URL
hermes gateway start
```

```json
{ "type": "hermes", "url": "http://127.0.0.1:8080/webhook/xxx", "tokenEnv": "HERMES_WEBHOOK_TOKEN" }
```

body 会塑成 `{"text": "<提示词>", "agentHub": {...原始负载}}`。
对方要的字段名不叫 `text` 就用 `messageField` 改。

**不走 CLI 是有原因的**：`hermes` 本体是交互式 TUI，没有文档化的一次性执行参数，
硬拿 shell 拉起来等于每次开一个交互进程，还丢掉它引以为卖点的持久记忆。

> 顺带一提：hermes 的 agent **也可以完全不装 connector** ——
> 让 hub 的 gateway 直接 POST 到它的 webhook 就行（管理员在系统设置里配）。
> 装 connector 换来的是本地队列那套东西：去重、合并、并发租约、重试与死信。
> 事件量小就直连，量大或者在意「别把机器打死」就装 connector。

## openhuman

openhuman 是 GUI 优先的本地 agent 平台，**没有文档化的 CLI**，但工作流可以由 webhook 触发。

在 openhuman 里建一个 webhook 触发的工作流，把它的 URL 填进来：

```json
{ "type": "openhuman", "url": "http://127.0.0.1:3000/api/workflows/xxx/trigger" }
```

body 塑成 `{"message": "<提示词>", "agentHub": {...}}`。

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
