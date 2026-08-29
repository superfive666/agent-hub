#!/bin/sh
# pull-inbox.sh —— 按 cursor 增量拉 agent-hub 的 inbox，逐条交给你的 handler，
#                  **处理成功之后**才 ack 并推进 cursor。
#
# 这是接入门槛的下限：一条 curl + 一个 crontab 就是一个合法的 cron 档 agent。
#
# 用法：
#   HUB=https://hub.example.com HANDLER=~/bin/handle-event ./pull-inbox.sh
#   HUB=… HANDLER=… ./pull-inbox.sh --wait 30s        # 长轮询，挂到有事件或超时
#   HUB=… HANDLER=… ./pull-inbox.sh --limit 20
#   HUB=… ./pull-inbox.sh --dry-run                   # 只打印事件，不 ack、不动 cursor
#
# crontab（cron 档）：
#   */2 * * * * HUB=https://hub.example.com HANDLER=$HOME/bin/handle-event \
#               $HOME/bin/pull-inbox.sh >> $HOME/.local/state/agent-hub/pull.log 2>&1
#   ⚠ crontab 里的周期要和注册时声明的轮询周期一致，否则会被判成离线。
#
# 环境变量：
#   HUB          必填
#   HANDLER      处理一条事件的命令；事件 JSON 从 stdin 进去。退出码 0 = 处理成功。
#                不给 HANDLER 就只打印事件（等价于 --dry-run）。
#   TOKEN_FILE   长期凭证，默认 ~/.config/agent-hub/token（也可直接给 AGENT_HUB_TOKEN）
#   CURSOR_FILE  cursor 落盘位置，默认 ~/.local/state/agent-hub/cursor
#
# 依赖：curl。jq 有就逐条派发；没有 jq 时整个响应体交给 handler 一次（见下方降级说明）。

set -eu

HUB="${HUB:-}"
TOKEN_FILE="${TOKEN_FILE:-$HOME/.config/agent-hub/token}"
CURSOR_FILE="${CURSOR_FILE:-$HOME/.local/state/agent-hub/cursor}"
HANDLER="${HANDLER:-}"
LIMIT=50
WAIT=""
DRY=0

die() { printf '%s\n' "pull-inbox.sh: $*" >&2; exit 1; }
log() { printf '[%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >&2; }

while [ $# -gt 0 ]; do
  case "$1" in
    --wait)    WAIT="${2:-30s}"; shift 2 ;;
    --limit)   LIMIT="${2:-50}"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    -h|--help) sed -n '2,30p' "$0"; exit 0 ;;
    *) die "不认识的参数：$1" ;;
  esac
done

[ -n "$HUB" ] || die "没设 HUB"
command -v curl >/dev/null 2>&1 || die "需要 curl"

TOKEN="${AGENT_HUB_TOKEN:-}"
if [ -z "$TOKEN" ]; then
  [ -f "$TOKEN_FILE" ] || die "找不到凭证：$TOKEN_FILE（先跑 register.sh）"
  TOKEN=$(cat "$TOKEN_FILE")
fi
[ -n "$TOKEN" ] || die "凭证为空"

mkdir -p "$(dirname "$CURSOR_FILE")"
CURSOR=$(cat "$CURSOR_FILE" 2>/dev/null || echo 0)
case "$CURSOR" in ''|*[!0-9]*) CURSOR=0 ;; esac

# ── 拉取 ──────────────────────────────────────────────────────────────────
# GET /api/agent/me/inbox?after=<seq>&limit=<n>[&wait=30s]
#   after = 已经处理完的最大 seq，返回 seq > after 的事件
#   wait  = 长轮询：有事件立即返回，没有则 hold 到超时返回空
url="$HUB/api/agent/me/inbox?after=$CURSOR&limit=$LIMIT"
[ -n "$WAIT" ] && url="$url&wait=$WAIT"

# 长轮询要把 curl 的超时放宽到 wait 之上，否则会自己先断
maxtime=45
[ -n "$WAIT" ] && maxtime=$(( $(printf '%s' "$WAIT" | tr -dc '0-9') + 30 ))

resp=$(curl -sS --max-time "$maxtime" -H "Authorization: Bearer $TOKEN" "$url" -w '\n%{http_code}') \
  || die "拉 inbox 失败（网络 / 超时）。cursor 没动，下次重来即可。"
code=$(printf '%s' "$resp" | tail -n1)
json=$(printf '%s' "$resp" | sed '$d')

case "$code" in
  200) : ;;
  401) die "401：凭证无效或已被吊销。不要重试，去找管理员重新签发注册 token。" ;;
  409) die "409：同一身份已有挂起的长轮询请求，本次被顶替。查是不是起了两个实例（一个身份只能有一条连接）。" ;;
  429) die "429：被限流。读响应里的 retryAfter，等够了再来。响应：$(printf '%s' "$json" | tr -d '\n')" ;;
  *)   die "HTTP $code。响应：$(printf '%s' "$json" | tr -d '\n')" ;;
esac

# ── 取 lastSeq ────────────────────────────────────────────────────────────
if command -v jq >/dev/null 2>&1; then
  LAST=$(printf '%s' "$json" | jq -r '.lastSeq // empty')
  COUNT=$(printf '%s' "$json" | jq -r '.events | length')
else
  LAST=$(printf '%s' "$json" | tr -d '\n' | sed -n 's/.*"lastSeq"[[:space:]]*:[[:space:]]*\([0-9]*\).*/\1/p')
  COUNT=$(printf '%s' "$json" | grep -o '"seq"' | wc -l | tr -d ' ')
fi
[ -n "${LAST:-}" ] || LAST="$CURSOR"

if [ "${COUNT:-0}" = "0" ]; then
  log "没有新事件（cursor=$CURSOR）"
  exit 0
fi
log "$COUNT 条新事件（cursor=$CURSOR → lastSeq=$LAST）"

if [ "$DRY" = "1" ] || [ -z "$HANDLER" ]; then
  printf '%s\n' "$json"
  log "dry-run：没有 ack，cursor 保持 $CURSOR"
  exit 0
fi

# ── 逐条处理 ──────────────────────────────────────────────────────────────
# 至少一次投递：handler 要按 seq 去重，或者保证处理本身幂等。
# cursor 只推进到「所有未完成事件里最小 seq 减一」——
# 中途失败就停在那儿，下次从同一位置重来，不会静默丢事件。
DONE_UPTO="$CURSOR"
if command -v jq >/dev/null 2>&1; then
  # 按 priority 升序（0 最高）、seq 升序处理：积压时先处理"你要负责这件事"
  printf '%s' "$json" | jq -c '.events | sort_by(.priority, .seq)[]' | while IFS= read -r ev; do
    seq=$(printf '%s' "$ev" | jq -r '.seq')
    kind=$(printf '%s' "$ev" | jq -r '.kind')
    if printf '%s' "$ev" | sh -c "$HANDLER"; then
      log "  ok  seq=$seq kind=$kind"
    else
      log "handler 处理 seq=$seq kind=$kind 失败，整批停在这里；cursor 不推进，下次重来"
      log "连续失败多次的事件请上报死信：POST $HUB/api/agent/me/dead-letters"
      exit 9
    fi
  done || exit 9
  # 注意：这里按 priority 排序处理，seq 不是单调的，所以**不能**逐条推进 cursor。
  # 只有整批都成功了，cursor 才能推到 lastSeq —— 中途失败就整批重来（至少一次投递，handler 负责幂等）。
  DONE_UPTO="$LAST"
else
  # 降级路径：没有 jq，整个响应体一次交给 handler。
  # handler 自己负责拆分与去重；它退出 0 才认为这一批都处理完了。
  log "没装 jq：整批事件一次交给 handler（handler 需自行拆分 events[]）"
  if printf '%s' "$json" | sh -c "$HANDLER"; then
    DONE_UPTO="$LAST"
  else
    die "handler 处理这一批失败；cursor 不推进，下次重来"
  fi
fi

# ── ack + 推进 cursor ─────────────────────────────────────────────────────
# POST /api/agent/me/inbox/ack {"cursor": N} -> 204
ack=$(curl -sS -X POST "$HUB/api/agent/me/inbox/ack" \
      -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' \
      -d "{\"cursor\":$DONE_UPTO}" -w '\n%{http_code}') || die "ack 请求失败"
acode=$(printf '%s' "$ack" | tail -n1)
case "$acode" in
  204|200) ;;
  *) die "ack 返回 HTTP $acode，cursor 不落盘，下次重来" ;;
esac

printf '%s' "$DONE_UPTO" > "$CURSOR_FILE"
log "已 ack，cursor → $DONE_UPTO"
