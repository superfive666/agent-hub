#!/usr/bin/env bash
# 一条命令装完 agent-hub connector：装成 user service（不需要 root）、写配置、验证连通性。
set -euo pipefail

APP=agent-hub-connector
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONF_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/$APP"
DATA_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/$APP"
STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/$APP"
UNIT_DIR="$HOME/.config/systemd/user"

say() { printf '\033[1m==>\033[0m %s\n' "$*"; }
die() { printf '\033[31m!!\033[0m %s\n' "$*" >&2; exit 1; }

command -v node >/dev/null || die "需要 Node 22+，没找到 node"
NODE_MAJOR=$(node -p 'process.versions.node.split(".")[0]')
[ "$NODE_MAJOR" -ge 22 ] || die "需要 Node 22+，当前 $(node -v)"

say "构建"
(cd "$SRC" && npm install --no-audit --no-fund && npm run build)

say "安装到 $DATA_DIR"
mkdir -p "$DATA_DIR" "$CONF_DIR" "$STATE_DIR"
chmod 700 "$CONF_DIR" "$STATE_DIR"
rm -rf "$DATA_DIR/dist" "$DATA_DIR/node_modules"
cp -r "$SRC/dist" "$DATA_DIR/dist"
cp "$SRC/package.json" "$DATA_DIR/package.json"

if [ ! -f "$CONF_DIR/config.json" ]; then
  cp "$SRC/config.example.json" "$CONF_DIR/config.json"
  say "已生成 $CONF_DIR/config.json —— 先把 hub.baseUrl / agentId / adapter 改对再继续"
fi

# 凭证：只放在 0600 的环境文件里，绝不进 unit、绝不进 config
if [ ! -f "$CONF_DIR/env" ]; then
  umask 077
  printf 'AGENT_HUB_TOKEN=%s\n' "${AGENT_HUB_TOKEN:-把注册时拿到的长期凭证填这里}" > "$CONF_DIR/env"
fi
chmod 600 "$CONF_DIR/env"

say "写 systemd user unit"
mkdir -p "$UNIT_DIR"
sed "s#%h#$HOME#g" "$SRC/systemd/$APP.service" > "$UNIT_DIR/$APP.service"

say "验证配置、凭证与 hub 连通性"
set +e
( set -a; . "$CONF_DIR/env"; set +a
  node --experimental-sqlite "$DATA_DIR/dist/src/index.js" check --config "$CONF_DIR/config.json" )
CHECK=$?
set -e
[ $CHECK -eq 0 ] || die "连通性检查没过。改完 $CONF_DIR/config.json 和 $CONF_DIR/env 再跑一次 ./install.sh"

say "启动服务"
systemctl --user daemon-reload
systemctl --user enable --now "$APP.service"
# 让服务在你登出后继续活着（否则 systemd 会在最后一个会话结束时杀掉 user manager）
loginctl enable-linger "$(id -un)" 2>/dev/null || say "enable-linger 失败（可能没权限）：登出后 connector 会停"

sleep 1
systemctl --user --no-pager --lines=10 status "$APP.service" || true
say "装好了。日志：journalctl --user -u $APP -f"
