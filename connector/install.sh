#!/bin/sh
# 一条命令装完 agent-hub connector：装成 user service（不需要 root）、写配置、验证连通性。
#
# **POSIX sh，不是 bash。** onboard.sh 用 `sh install.sh` 调它，仓库里其它脚本
# （register.sh / pull-inbox.sh / card.sh）也都是 #!/bin/sh。Ubuntu 的 /bin/sh 是
# dash，bash 语法在这里不是「大概能跑」而是当场死：`set -o pipefail` 在 dash 里
# 直接报 `Illegal option -o pipefail` 退出 2，连第 6 行都到不了。
set -eu

APP=agent-hub-connector
SRC=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
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

# 凭证：正常路径是 config.json 里的 tokenFile（register.sh 已经 0600 落盘），
# env 文件只服务于「凭证从环境变量来」的场景（容器、CI）。
#
# **这里绝不写占位符。** connector 的 readToken 是 env 优先、文件兜底，env 里塞一个
# 假值会把 tokenFile 里的真凭证整个盖掉 —— 症状是注册明明成功、token 文件明明是对的，
# 但每一发都 401。这种失败没有任何一条日志会指向这个文件。
PLACEHOLDER='把注册时拿到的长期凭证填这里'
if [ -f "$CONF_DIR/env" ] && grep -q "^AGENT_HUB_TOKEN=$PLACEHOLDER\$" "$CONF_DIR/env"; then
  # 老版本装出来的占位文件。留着它比没有它更糟，直接清掉。
  say "清掉 $CONF_DIR/env 里的占位凭证（它会盖住 tokenFile 里的真凭证）"
  rm -f "$CONF_DIR/env"
fi
if [ -n "${AGENT_HUB_TOKEN:-}" ] && [ ! -f "$CONF_DIR/env" ]; then
  (umask 077; printf 'AGENT_HUB_TOKEN=%s\n' "$AGENT_HUB_TOKEN" > "$CONF_DIR/env")
fi
if [ -f "$CONF_DIR/env" ]; then chmod 600 "$CONF_DIR/env"; fi

say "写 systemd user unit"
mkdir -p "$UNIT_DIR"
# unit 里的路径写死成本次安装真正用的目录。unit 模板用 %h 占位，而 %h 只等于 $HOME，
# 表达不了 XDG_CONFIG_HOME / XDG_DATA_HOME —— 那两个一被设过，服务就会去读一组
# 根本没装东西的路径。所以按目录整段替换，而不是只把 %h 换成 $HOME。
sed -e "s#%h/.config/$APP#$CONF_DIR#g" \
    -e "s#%h/.local/share/$APP#$DATA_DIR#g" \
    "$SRC/systemd/$APP.service" > "$UNIT_DIR/$APP.service"

say "验证配置、凭证与 hub 连通性"
set +e
( set -a
  if [ -f "$CONF_DIR/env" ]; then . "$CONF_DIR/env"; fi
  set +a
  node --experimental-sqlite "$DATA_DIR/dist/src/index.js" check --config "$CONF_DIR/config.json" )
CHECK=$?
set -e
[ $CHECK -eq 0 ] || die "连通性检查没过。改完 $CONF_DIR/config.json 再跑一次 sh install.sh"

say "启动服务"
systemctl --user daemon-reload
systemctl --user enable --now "$APP.service"
# 让服务在你登出后继续活着（否则 systemd 会在最后一个会话结束时杀掉 user manager）
loginctl enable-linger "$(id -un)" 2>/dev/null || say "enable-linger 失败（可能没权限）：登出后 connector 会停"

sleep 1
systemctl --user --no-pager --lines=10 status "$APP.service" || true
say "装好了。日志：journalctl --user -u $APP -f"
