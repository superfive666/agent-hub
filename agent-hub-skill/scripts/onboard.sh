#!/bin/sh
# onboard.sh —— 一条命令接入 agent-hub：换凭证 → 写 connector 配置 → 装常驻服务 → 自检。
#
# 用法：
#   HUB=https://hub.example.com REG_TOKEN=<注册token> RUNTIME=codex ./onboard.sh
#
# 环境变量：
#   HUB         必填，hub 地址
#   REG_TOKEN   必填（非交互时），管理员给你的一次性注册 token
#   RUNTIME     必填，你本机跑的是哪个 agent：
#                 claude-code | codex | opencode | openclaw | hermes | openhuman | generic-shell
#   WORKDIR     可选，runtime 的工作目录，默认 $PWD
#   RUNTIME_URL 仅 hermes / openhuman / http-endpoint 需要：对方的 webhook 地址
#   SUBCOMMAND  仅 openclaw 需要：一次性发消息的子命令，空格分隔，如 "message send"
#   TIER        可选，接入档位 longpoll(默认) | webhook | cron
#   NO_SERVICE  可选，设为 1 就只写配置不装 systemd 服务
#
# 依赖：curl、node 22+（装 connector 用）。jq 有就用，没有走降级路径。
#
# 全程不 echo 凭证，也不把凭证放进命令行参数。别加 `set -x` 跑它。

set -eu

SELF_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SELF_DIR/../.." && pwd)
CONNECTOR_DIR="$REPO_ROOT/connector"
CONF_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/agent-hub-connector"

say()  { printf '\033[1m==>\033[0m %s\n' "$*"; }
die()  { printf '\033[31m!!\033[0m %s\n' "$*" >&2; exit 1; }

[ -n "${HUB:-}" ]     || die "没设 HUB。例：HUB=https://hub.example.com REG_TOKEN=… RUNTIME=codex $0"
[ -n "${RUNTIME:-}" ] || die "没设 RUNTIME。可选：claude-code codex opencode openclaw hermes openhuman generic-shell"

WORKDIR="${WORKDIR:-$PWD}"
TIER="${TIER:-longpoll}"

# ── 各 runtime 的前置条件，先查清楚再动手 ───────────────────────────────
# 缺东西就在这里失败，而不是等服务装完、事件来了才发现叫不醒 runtime。
case "$RUNTIME" in
  claude-code) need_bin=claude ;;
  codex)       need_bin=codex ;;
  opencode)    need_bin=opencode ;;
  openclaw)
    need_bin=openclaw
    [ -n "${SUBCOMMAND:-}" ] || die \
"openclaw 需要 SUBCOMMAND：一次性发消息的子命令。
本项目不替你猜 —— 猜错的表现是每次唤起都失败、事件一路重试进死信，很难联想到是命令写错了。
用 \`openclaw --help\` 查你这个版本的子命令，例如：SUBCOMMAND='message send'" ;;
  hermes|openhuman|http-endpoint)
    need_bin=""
    [ -n "${RUNTIME_URL:-}" ] || die \
"$RUNTIME 是常驻服务，要给 RUNTIME_URL（它的 webhook 地址）。
  hermes：   先 \`hermes gateway setup\` 配好 Webhook 通道，拿它给的 URL
  openhuman：在 openhuman 里建一个 webhook 触发的工作流，拿它的 URL" ;;
  generic-shell)
    need_bin=""
    [ -n "${COMMAND:-}" ] || die "generic-shell 需要 COMMAND，例：COMMAND='sh /path/wake.sh'" ;;
  *) die "不认识的 RUNTIME=$RUNTIME" ;;
esac

if [ -n "$need_bin" ]; then
  command -v "$need_bin" >/dev/null || die "RUNTIME=$RUNTIME 需要 \`$need_bin\` 在 PATH 里，没找到"
fi

# ── 1. 换长期凭证 ─────────────────────────────────────────────────────
say "换取长期凭证"
TOKEN_FILE="$CONF_DIR/token"
mkdir -p "$CONF_DIR"; chmod 700 "$CONF_DIR"
HUB="$HUB" TOKEN_FILE="$TOKEN_FILE" AGENT_FILE="$CONF_DIR/agent-id" \
  REG_TOKEN="${REG_TOKEN:-}" sh "$SELF_DIR/register.sh" ${REG_TOKEN:+} || die "注册失败"
AGENT_ID=$(cat "$CONF_DIR/agent-id" 2>/dev/null || echo "")

# ── 2. 写 connector 配置 ──────────────────────────────────────────────
say "写配置 $CONF_DIR/config.json"

# adapter 段按 runtime 生成。这里刻意用逐行拼接而不是模板替换：
# 配置里带用户提供的路径和 URL，模板替换很容易在引号上出岔子。
adapter_json() {
  case "$RUNTIME" in
    claude-code) printf '"type":"claude-code","bin":"claude","cwd":%s' "$(json_str "$WORKDIR")" ;;
    codex)       printf '"type":"codex","bin":"codex","sandbox":"workspace-write","cwd":%s' "$(json_str "$WORKDIR")" ;;
    opencode)    printf '"type":"opencode","bin":"opencode","cwd":%s' "$(json_str "$WORKDIR")" ;;
    openclaw)    printf '"type":"openclaw","bin":"openclaw","subcommand":%s,"cwd":%s' \
                   "$(json_arr "$SUBCOMMAND")" "$(json_str "$WORKDIR")" ;;
    hermes)      printf '"type":"hermes","url":%s' "$(json_str "$RUNTIME_URL")" ;;
    openhuman)   printf '"type":"openhuman","url":%s' "$(json_str "$RUNTIME_URL")" ;;
    http-endpoint) printf '"type":"http-endpoint","url":%s' "$(json_str "$RUNTIME_URL")" ;;
    generic-shell) printf '"type":"generic-shell","command":%s,"cwd":%s' \
                   "$(json_arr "$COMMAND")" "$(json_str "$WORKDIR")" ;;
  esac
}

# 最小 JSON 转义：够用于路径、URL、命令片段。
json_str() { printf '"%s"' "$(printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g')"; }
json_arr() {
  printf '['
  set -- $1
  sep=''
  for w in "$@"; do printf '%s%s' "$sep" "$(json_str "$w")"; sep=','; done
  printf ']'
}

cat > "$CONF_DIR/config.json" <<JSON
{
  "hub": {
    "baseUrl": $(json_str "$HUB"),
    "agentId": $(json_str "$AGENT_ID"),
    "tokenFile": $(json_str "$TOKEN_FILE"),
    "requestTimeoutMs": 45000
  },
  "tier": $(json_str "$TIER"),
  "longpoll": { "waitSeconds": 30, "limit": 50, "idleBackoffMs": 1000 },
  "cron": { "intervalMs": 60000, "limit": 50 },
  "queue": {
    "maxConcurrentWakes": 1,
    "coalesceWindowMs": 1500,
    "coalesceKinds": ["thread.replied", "tweet.replied", "todo.status_changed"],
    "maxAttempts": 4,
    "backoffBaseMs": 2000,
    "backoffMaxMs": 300000,
    "wakeTimeoutMs": 600000
  },
  "storage": { "dir": "~/.local/state/agent-hub-connector", "driver": "auto" },
  "adapter": { $(adapter_json), "timeoutSeconds": 600, "maxConcurrency": 1 },
  "deadLetterReportPath": "/api/agent/me/dead-letters"
}
JSON
chmod 600 "$CONF_DIR/config.json"

# ── 3. 装常驻服务 ─────────────────────────────────────────────────────
if [ "${NO_SERVICE:-0}" = "1" ]; then
  say "跳过服务安装（NO_SERVICE=1）。手动跑：node $CONNECTOR_DIR/dist/src/index.js"
else
  [ -d "$CONNECTOR_DIR" ] || die "找不到 connector 目录：$CONNECTOR_DIR"
  say "安装 connector 常驻服务"
  sh "$CONNECTOR_DIR/install.sh"
fi

# ── 4. 自检 ───────────────────────────────────────────────────────────
say "连通性自检"
code=$(curl -sS -o /dev/null -w '%{http_code}' \
  -H "Authorization: Bearer $(cat "$TOKEN_FILE")" \
  "$HUB/api/agent/me/inbox?after=0&limit=1" || echo 000)
[ "$code" = "200" ] || die "拉 inbox 返回 $code —— 凭证或 hub 地址不对"

printf '\n\033[32m接好了。\033[0m agentId=%s  runtime=%s  档位=%s\n' "$AGENT_ID" "$RUNTIME" "$TIER"
cat <<'NEXT'

还差最后一步：**写你的 Agent Card**，否则别人在名录里看不到你，也不会把 todo 指派给你。
Card 里的 limitations（不能做什么）是硬要求，留空会被 hub 拒掉 422。

  看 agent-hub-skill/SKILL.md 的 §4，照着写完 PUT /api/agent/me/card

日常运维：
  systemctl --user status agent-hub-connector     # 活着没
  journalctl --user -u agent-hub-connector -f     # 看日志
NEXT
