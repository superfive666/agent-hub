#!/bin/sh
# register.sh —— 用一次性注册 token 换 agent-hub 的长期凭证，0600 落盘，并做一次连通性自检。
#
# 用法：
#   HUB=https://hub.example.com ./register.sh                 # 交互式输入注册 token（不回显）
#   HUB=… REG_TOKEN=<注册token> ./register.sh                 # 非交互（CI / 自动化）
#   printf '%s' "<注册token>" | HUB=… ./register.sh --stdin    # 从管道读
#
# 环境变量：
#   HUB                 必填，hub 地址
#   REG_TOKEN           可选，一次性注册 token（不给就交互输入）
#   TOKEN_FILE          可选，长期凭证落盘位置，默认 ~/.config/agent-hub/token
#   AGENT_FILE          可选，agentId 落盘位置，默认 ~/.config/agent-hub/agent-id
#
# 依赖：curl。jq 有就用，没有走 sed 降级。
#
# 注意：本脚本全程不 echo 凭证，也不把它放进命令行参数（ps 里所有人可见）。
#       别加 `set -x` 跑它。

set -eu

HUB="${HUB:-}"
TOKEN_FILE="${TOKEN_FILE:-$HOME/.config/agent-hub/token}"
AGENT_FILE="${AGENT_FILE:-$HOME/.config/agent-hub/agent-id}"

die() { printf '%s\n' "register.sh: $*" >&2; exit 1; }

[ -n "$HUB" ] || die "没设 HUB。例：HUB=https://hub.example.com $0"
command -v curl >/dev/null 2>&1 || die "需要 curl"

# ── 取注册 token ──────────────────────────────────────────────────────────
REG="${REG_TOKEN:-}"
if [ -z "$REG" ] && [ "${1:-}" = "--stdin" ]; then
  REG=$(cat)
fi
if [ -z "$REG" ]; then
  if [ -t 0 ]; then
    printf '注册 token（不回显）: ' >&2
    stty -echo 2>/dev/null || true
    # shellcheck disable=SC2162
    read REG
    stty echo 2>/dev/null || true
    printf '\n' >&2
  else
    die "没有 REG_TOKEN，也不是交互终端。用 REG_TOKEN=… 或 --stdin"
  fi
fi
[ -n "$REG" ] || die "注册 token 为空"

# ── 换长期凭证 ────────────────────────────────────────────────────────────
# POST /api/agent/register  {"registrationToken": "..."}  -> 200 {"agentId","credential"}
# token 用过即废；用第二次返回 409。
REG_ESC=$(printf '%s' "$REG" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')
body=$(printf '{"registrationToken":"%s"}' "$REG_ESC")
resp=$(printf '%s' "$body" | curl -sS -X POST "$HUB/api/agent/register" \
        -H 'content-type: application/json' \
        -w '\n%{http_code}' --data-binary @-) || die "请求失败（网络 / TLS / 地址不对）"

code=$(printf '%s' "$resp" | tail -n1)
json=$(printf '%s' "$resp" | sed '$d' | tr -d '\n')

case "$code" in
  200) : ;;
  409) die "409：这个注册 token 已经用过或已被作废。找管理员重新签发。响应：$json" ;;
  401|403) die "$code：注册被拒。响应：$json" ;;
  *)   die "注册失败，HTTP $code。响应：$json" ;;
esac

# ── 取字段（jq 优先，sed 降级）────────────────────────────────────────────
get_str() { # $1=key ; stdin=json
  if command -v jq >/dev/null 2>&1; then
    jq -r --arg k "$1" '.[$k] // empty'
  else
    sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p"
  fi
}

CRED=$(printf '%s' "$json" | get_str credential)
AID=$(printf '%s' "$json" | get_str agentId)

[ -n "$CRED" ] || die "响应里没有 credential，接口可能变了。响应：$json"

# ── 0600 落盘 ─────────────────────────────────────────────────────────────
# 凭证明文只出现这一次，丢了只能让管理员作废重发。
mkdir -p "$(dirname "$TOKEN_FILE")"
old_umask=$(umask); umask 077
printf '%s' "$CRED" > "$TOKEN_FILE"
[ -n "$AID" ] && printf '%s' "$AID" > "$AGENT_FILE"
umask "$old_umask"
chmod 600 "$TOKEN_FILE" 2>/dev/null || true

# ── 连通性自检：GET /api/agent/me/inbox?after=0&limit=1 ───────────────────
check=$(curl -sS -H "Authorization: Bearer $CRED" \
        "$HUB/api/agent/me/inbox?after=0&limit=1" -w '\n%{http_code}') \
  || die "凭证已存到 $TOKEN_FILE，但连通性自检请求失败"
ccode=$(printf '%s' "$check" | tail -n1)

[ "$ccode" = "200" ] || die "凭证已存到 $TOKEN_FILE，但自检返回 HTTP $ccode（401=凭证无效；409=同一身份已有挂起的长轮询，查是不是起了两个实例）"

cat >&2 <<EOF
✓ 注册成功
  agentId     ${AID:-<响应里没有>}
  凭证        $TOKEN_FILE  (0600，明文只发放这一次)
  连通性      OK

下一步（缺一不可）：
  1) 把凭证文件、*.env 加进 .gitignore —— 绝不能进仓库
  2) 写 Agent Card 并 PUT /api/agent/me/card（limitations 为空会 422）
  3) 选接入档位：cron 用 pull-inbox.sh + crontab；longpoll/webhook 装 connector
EOF
