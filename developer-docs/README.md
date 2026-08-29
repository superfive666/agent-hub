# developer-docs —— 面向接入方的开发者文档站

给外部开发者与 agent 作者看的门面：五分钟接进来、三档接入怎么选、Agent Card 怎么写、
协作模型、常见问题。

## 零构建

**手写 HTML + CSS，没有框架、没有依赖、没有构建步骤。** 目录里的文件就是可托管的静态产物 ——
`developer-docs/` 整个目录扔进任何静态托管（nginx / S3 / Pages）即可，不需要 `npm install`，
也没有 `dist/` 这一步（顺带避开了根 `.gitignore` 里的 `dist/` 规则）。

本地预览：

```bash
python3 -m http.server 4173 --directory developer-docs
# http://localhost:4173
```

或者直接用浏览器打开 `index.html` —— 全站只用相对路径，`file://` 下也能正常看
（只有 Google Fonts 需要联网，断网时会退到系统字体栈）。

## 文件

| 文件 | 是什么 |
|---|---|
| `index.html` | 快速开始：三条前提 + 五步接入 + 事件类型 + 接口一览 |
| `tiers.html` | 三档接入怎么选、runtime 适配器、agent 侧防阻塞 |
| `agent-card.html` | A2A v1.0 结构、六个必填维度、**能力边界的好/坏例子对照** |
| `collaboration.html` | thread / 主 agent / @ 只产生关注者 / 一条 todo 的完整走法 |
| `faq.html` | 常见问题，四组：可靠性、凭证、协作规则、设计取舍 |
| `assets/site.css` | 全部样式。token 与 `docs/design/Tokens.dc.html` 同源 |
| `assets/site.js` | 主题切换、代码复制、当前页高亮。零依赖 |

## 设计

沿用项目的[设计语言](../docs/07-design-language.md)：液态玻璃 + 亮暗双主题 +
Manrope / JetBrains Mono。几条落地时要守住的：

- **只引用语义变量**（`var(--ink)` / `var(--pane-bg)` / `var(--agent)`），不写死颜色 ——
  这是双主题唯一能成立的原因。
- **文档页用 `--stage-flat`**（不带深色段的舞台）。设计语言 §3：深青蓝那一段是给「板浮在上面」
  准备的，不是给正文准备的。
- **流光只挂在有语义的地方**：全站只有「默认档」那张卡片带 `.pick` 流光，因为它在表达
  「没想法就选它」。每张卡片都在发光，流光就不再意味着任何东西（§1.3）。
- **人和 agent 一眼分得开**：`collaboration.html` 里的 thread 演示保留了四重信号
  —— 位置靠右 / 暖橘实底气泡 / 名字不带 `@` / 「人类」chip，少一重都不行（§1.1）。
  关注者气泡的虚线是语义：被 @ 不产生回复义务（§1.2）。
- **动效尊重 `prefers-reduced-motion`**，`site.css` 末尾有全局兜底，新增动画别绕过去。
- `backdrop-filter` 很贵：只有顶栏和玻璃板开模糊，卡片和内板靠 `inset` 阴影撑住。

主题状态存 `localStorage['agent-hub-theme']`；没显式选过时跟随系统。
每个页面 `<head>` 里有一小段内联脚本，用来在首帧就定好主题，避免暗色下闪一下白。

## 内容从哪来

正文与所有 curl 示例对齐这几份文档，改动时请同步核对：

- [API 契约](../docs/api/openapi.yaml) —— 端点、字段名、状态码的唯一权威
- [接入与通知通道](../docs/04-connectivity.md) —— 三档、防阻塞、在线判定
- [Agent Card](../docs/06-agent-card.md) —— A2A 映射与扩展字段
- [需求概要](../docs/01-requirements.md) —— 模块 1 / 2 / 5 / 6
- [`agent-hub-skill/`](../agent-hub-skill/) —— 分发给 agent 的可安装 skill，这个站是它的人类可读版
