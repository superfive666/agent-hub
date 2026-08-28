# web —— agent-hub 管理控制台

Vite + React 19 + TypeScript + Tailwind v4 + React Router 7 + TanStack Query。
UI 走 shadcn//ui 的路子：Radix primitives + 自己控制样式，组件源码在 `src/components/ui/`。
**没有也不要加 MUI / Ant Design 之类的第三方 UI 库。**

## 跑起来

```bash
node -v            # 需要 Node 22
npm install
npm run dev        # http://localhost:5173，/api 代理到 localhost:8080
npm run build      # tsc -b && vite build
npm run test       # Vitest
npm run gen:api    # 从 ../docs/api/openapi.yaml 重新生成 src/api/schema.d.ts
```

对话页在 `/threads`（根路径会重定向过去）。当前用 `src/mocks/thread.ts` 里的静态假数据。

## 目录

| 路径 | 是什么 |
|---|---|
| `src/styles/theme.css` | Token 层。语义变量、厚玻璃六层阴影、棱镜边、高光扫过、动效关键帧。亮暗两套 |
| `src/components/ui/` | 基础组件：Button / Chip / Avatar / Card / Pane / Inset / Seg / Bubble |
| `src/components/outbox-alert.tsx` | outbox 滞后告警。**不可折叠、不可降级**（设计语言 §1.4） |
| `src/routes/thread.tsx` | 对话页：舞台 → 玻璃板 → 嵌套内板 |
| `src/hooks/useTheme.ts` | 主题切换。`.dark` 挂 `<html>`，存 localStorage，默认跟随系统 |
| `src/hooks/useInboxStream.ts` | inbox 长轮询 |
| `src/api/schema.d.ts` | **生成产物，不要手改**。改契约请改 `docs/api/openapi.yaml` 后跑 `npm run gen:api` |

## 改样式之前

先读 [设计语言](../docs/07-design-language.md) 和 [web/.claude/skills/agent-hub-design](.claude/skills/agent-hub-design/SKILL.md)。
五条不变量是功能，不是审美偏好。几条最容易踩的：

- **只引用语义变量**（`var(--agent)` / `var(--pane-bg)` / `var(--ink2)`），不写死颜色。
  这是亮暗双主题唯一能成立的原因。
- **流光（`.glow` / `.runner`）只挂主 agent 卡片和当前会话。** 到处发光就等于没发光。
- `backdrop-filter` 很贵：同屏模糊层 ~8 个以内。**内板（`.inset`）和 `.card` 不再各自开模糊层**，
  它们靠 `inset` 阴影和低透明度背景撑住。
- 等宽字体只给真正的机器内容：agent id、seq、token、runtime 名、代码。

## 长轮询，不是定时轮询

`useInboxStream` 用 AbortController 跑一个循环：一个请求带 `?after=<cursor>&wait=30s` 挂在服务端，
有事件立即返回，没有则超时返回空，然后立刻用新 cursor 再发一个。

**不要换成 TanStack Query 的 `refetchInterval`** —— 那是固定间隔的定时轮询，语义不一样。

正确性只靠 cursor：断线后用同一个 cursor 重连，断开期间的事件在下一次响应里补齐；
`visibilitychange` / `online` 时会中断挂起的请求立刻补拉一次。推送信号可以丢，inbox 事件不能丢。

页面目前还没接这个 hook（数据是静态 mock），hook 与单测已就绪。
