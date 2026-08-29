# web —— agent-hub 管理控制台

Vite + React 19 + TypeScript + Tailwind v4 + React Router 7 + TanStack Query。
UI 走 shadcn//ui 的路子：Radix primitives + 自己控制样式，组件源码在 `src/components/ui/`。
**没有也不要加 MUI / Ant Design 之类的第三方 UI 库。**

## 跑起来

```bash
node -v            # 需要 Node 22
npm install
npm run dev        # http://localhost:5173，/api 代理到 localhost:8080
VITE_USE_MOCKS=1 npm run dev   # 后端没起时用假数据看界面
npm run build      # tsc -b && vite build
npm run test       # Vitest
npm run test:mocks # 单独跑 mock 模式的冒烟用例（要带 VITE_USE_MOCKS=1，不能和上面同一趟）
npm run gen:api    # 从 ../docs/api/openapi.yaml 重新生成 src/api/schema.d.ts
```

## 页面

| 路由 | 是什么 | 数据 |
|---|---|---|
| `/login` | 登录。会话是 HttpOnly Cookie，请求一律 `credentials: 'include'` | `POST /api/admin/login` |
| `/threads/:id` | 对话页。舞台 → 玻璃板 → 嵌套内板 | `GET /api/agent/threads/{id}` + inbox 长轮询 |
| `/board` | 看板。按天浏览，两种归档口径切换 | `GET /api/admin/board?date=&groupBy=` |
| `/todos` · `/todos/new` | Todo 列表与新建。主 agent 必选 | `GET/POST /api/admin/todos` |
| `/directory` | 名录。能力卡片网格，「不能做」用告警色块单独拎出来 | `GET /api/agent/directory` |
| `/settings` | 系统设置与运行状态 | `GET /api/admin/settings`、`GET /api/admin/health` |

除 `/login` 外全部包在 `RequireAuth` 里：`GET /api/admin/me` 返回 401 就跳登录页。
**401 才跳** —— 其它错误是"后端有问题"，冒充成"未登录"等于把故障藏起来。

## 目录

| 路径 | 是什么 |
|---|---|
| `src/styles/theme.css` | Token 层。语义变量、厚玻璃六层阴影、棱镜边、高光扫过、动效关键帧。亮暗两套 |
| `src/components/ui/` | 基础组件：Button / Chip / Avatar / Card / Pane / Inset / Seg / Bubble |
| `src/components/app-shell.tsx` | 舞台 + 两块玻璃板的底盘，所有页面共用 |
| `src/components/outbox-alert.tsx` · `outbox-banner.tsx` | outbox 滞后告警。**不可折叠、不可降级**（设计语言 §1.4），挂在每个页面顶部 |
| `src/components/mention-textarea.tsx` | 正文里的 `@` 提及下拉。**只产生关注者，不指派** |
| `src/api/schema.d.ts` | **生成产物，不要手改**。改契约请改 `docs/api/openapi.yaml` 后跑 `npm run gen:api` |
| `src/api/client.ts` | openapi-fetch 客户端 + 从 schema 导出的领域类型 + `HttpError` |
| `src/api/queries.ts` | TanStack Query 的 query/mutation 与 query key，mock 开关也在这 |
| `src/lib/format.ts` | 契约类型 → 展示模型：头像缩写、状态用语、时刻、进度 |
| `src/mocks/data.ts` | 假数据。**形状就是契约类型**，所以开关 mock 不会改变组件的代码路径 |
| `src/hooks/useInboxStream.ts` | inbox 长轮询 |
| `src/test/harness.tsx` | 测试用的 fetch 打桩 + 整棵路由树渲染 |

## 类型从契约来，不手写

`ThreadDetail` / `Post` / `TodoSummary` / `Settings` / `AgentSummary` 全部从
`docs/api/openapi.yaml` 生成。组件里需要的展示字段（头像缩写、状态中文、时刻）在
`src/lib/format.ts` 里推导，**不要往契约类型上贴展示字段**。

> ⚠️ 现在 `npm run gen:api` 会失败：`docs/api/openapi.yaml` 里 `/api/admin/todos`
> 这个 key 出现了两次（第 428 行的 GET 和第 528 行的 POST），YAML 重复键。
> 当前的 `src/api/schema.d.ts` 是把两段合并之后生成的。**契约那边把两个方法并到同一个
> key 下面之后，这个脚本就能直接跑。**

## 改样式之前

先读 [设计语言](../docs/07-design-language.md) 和 [web/.claude/skills/agent-hub-design](.claude/skills/agent-hub-design/SKILL.md)。
五条不变量是功能，不是审美偏好。几条最容易踩的：

- **只引用语义变量**（`var(--agent)` / `var(--pane-bg)` / `var(--ink2)`），不写死颜色。
  这是亮暗双主题唯一能成立的原因。
- **流光（`.glow` / `.runner`）只挂主 agent 卡片和当前会话。** 到处发光就等于没发光。
- **不要把 `.glow` 加到 `.pane` 上。** `.pane::before` 是棱镜边、`::after` 是高光扫过，
  两个伪元素都占满了；加上去会**静默**顶掉棱镜边。`src/test/thread.test.tsx` 里有一条用例盯着这个。
- `backdrop-filter` 很贵：同屏模糊层 ~8 个以内。**内板（`.inset`）和 `.card` 不再各自开模糊层**。
- 等宽字体只给真正的机器内容：agent id、seq、token、runtime 名、代码。
- **断点只有 640 和 1024**（设计语言 §4）。`theme.css` 里已经把 Tailwind 自带的
  `md` / `xl` / `2xl` 清掉了，只剩 `sm:` 与 `lg:` —— 别再引回 768/1280 那一套。

## 长轮询，不是定时轮询

`useInboxStream` 用 AbortController 跑一个循环：一个请求带 `?after=<cursor>&wait=30s` 挂在服务端，
有事件立即返回，没有则超时返回空，然后立刻用新 cursor 再发一个。

**不要换成 TanStack Query 的 `refetchInterval`** —— 那是固定间隔的定时轮询，语义不一样。

正确性只靠 cursor：断线后用同一个 cursor 重连，断开期间的事件在下一次响应里补齐；
`visibilitychange` / `online` 时会中断挂起的请求立刻补拉一次。推送信号可以丢，inbox 事件不能丢。

对话页已经接上了它：收到事件就 invalidate `['thread', id]` / `['todos']` / `['health']`，
由 TanStack Query 重新拉。**通知只负责快，正确性在重新拉的那一次。**
