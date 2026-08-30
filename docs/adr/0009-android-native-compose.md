# 0009. Android 客户端：原生 Kotlin + Jetpack Compose，重画而不是套壳

- **状态**：已接受
- **日期**：2026-08-30
- **关联**：[设计语言](../07-design-language.md) · [ADR-0007](0007-tech-stack.md) · [立项](../09-android-app.md)

## 背景

`web/` 的控制台已经同时适配桌面网页与 H5 移动端，用手机浏览器能用。
现在要多一个能装在手机上的 Android 客户端，要求是：

1. **前端风格和现有控制台几乎一致** —— 用的是同一套设计语言，不是另起一套；
2. **直连同一个 hub，登录方式一模一样** —— 不新增鉴权模型，不新增后端；
3. **它只是一个前端** —— 不存业务数据，不做本地决策，所有正确性仍然在 hub 那边。

约束来自 [设计语言](../07-design-language.md)：那套「厚玻璃」不是配色，是一整套
**依赖浏览器合成器特性**的做法 —— `backdrop-filter`、`conic-gradient` +
`mask-composite: exclude`、`mix-blend-mode: overlay`、注册过的
`@property --ang`。没有一条能原样搬到别的渲染栈上。

## 备选方案

| 方案 | 优点 | 代价 |
|------|------|------|
| A. WebView 壳（Capacitor / TWA） | 设计 100% 一致，增量工作量接近零；一份代码两处跑 | 本质是个浏览器：断网白屏、返回键/输入法/滚动全是网页手感；拿不到任何原生能力（系统通知、后台拉取、分享）；**用户装了个 app 却得到浏览器**，这正是他不装浏览器书签而去装 app 想避开的 |
| B. React Native + Expo | 能复用 `api/schema.d.ts` 与部分展示逻辑 | RN 同样没有 `backdrop-filter` / `conic-gradient` / `mix-blend-mode`，玻璃质感照样要靠 expo-blur + Skia 重做 —— **重画的量和 Compose 差不多，却要多背一个 JS 运行时和一条 bridge** |
| C. 原生 Kotlin + Compose | 真原生：手感、返回栈、输入法、系统主题、后台能力都是系统给的；后续要接推送/离线缓存不用换地基 | 那套玻璃要在 Compose 里从零重画一遍；设计还原度到不了像素级 |

## 决策

**选 C：原生 Kotlin + Jetpack Compose。**

关键判断是 **A 和 B 都省不掉重画，只有 A 能**，而 A 省掉重画的代价是交付一个浏览器。

- B 相对 C 的唯一优势是"复用"，但真正要复用的那部分（API 类型、状态枚举、
  中文文案）在 C 里也只是一次性抄写，**而玻璃质感在 B 里一样要重做**。
  为了复用几百行展示逻辑去背一个 JS 运行时，换算不过来。
- A 在「只要能看」的场景下是对的选择。但需求里写的是「重新写一个安卓的 APP」，
  而不是「把网页装起来」—— 一旦用 A，将来任何一条原生诉求（通知、离线、分享到
  hub）都要推倒重来。

### 「几乎一致」的判定标准

不追像素级还原，**追的是设计语言 §1 那五条不变量在 Android 上一条不少**。
它们是功能不是审美，所以是硬指标：

| 不变量 | 在 Android 上怎么落 |
|---|---|
| §1.1 人和 agent 一眼分得开 | 位置（右/左）+ 气泡（暖橘渐变 vs 玻璃面）+ `@` 前缀 + 「人类」chip，四重信号一重不少 |
| §1.2 主 agent 与关注者的层级 | 主 agent 虹彩渐变头像 + 双层辉光 + 呼吸缩放；关注者虚线描边、无辉光 |
| §1.3 特效只给有语义的地方 | 流光只挂主 agent 卡片与当前会话，别处一律不许 |
| §1.4 outbox 告警不可折叠 | 任何屏幕宽度下都在首屏，不进二级页 |
| §1.5 尊重 `prefers-reduced-motion` | 读系统「移除动画」开关（`Settings.Global.ANIMATOR_DURATION_SCALE`），为 0 时全局关掉动画 |

视觉底盘（舞台 → 玻璃板 → 嵌套内板、六层阴影、胶囊控件、Manrope + JetBrains Mono）
按 §2/§3 在 Compose 里重画，允许有肉眼可辨的差异，**不允许有语义上的差异**。

### 三个技术落点

- **玻璃模糊**：`Modifier.graphicsLayer { renderEffect = BlurEffect(...) }` 需要
  **API 31+**。低于 31 的机器**降级成不透明的高不透明度纯色面**，而不是"糊一层半透明白"
  —— 半透明白压在舞台的深青蓝上会把深色文字吃掉，那正是设计语言里
  「亮色第一版失败」的原因。降级路径必须保证可读性，不是保证像。
- **棱镜描边**：`Brush.sweepGradient` 铺满，再用 `BlendMode.DstOut` 掏掉中间，
  等价于 web 的 `mask-composite: exclude`。web 那条 `translateZ(0)` 的坑是
  Chromium 合成器特有的，Compose 不存在。
- **会话**：hub 的会话是 `HttpOnly` Cookie（`hub_session`），app **不另发 token**
  —— 用 OkHttp 的 `CookieJar` 存这一个 cookie，行为与浏览器一致。
  这样 `POST /api/admin/login` 一个端点不用改（见 [ADR-0010](0010-public-apk-download.md) 之外的说明）。
  OIDC 模式走**应用内 WebView**（不是 Custom Tabs）：Custom Tabs 的 cookie 落在
  浏览器进程里，app 拿不到；WebView 的 `CookieManager` 才能把 `hub_session` 交给 OkHttp。

## 影响

- 多一个技术栈（Kotlin / Gradle / Android SDK）和一条构建链。仓库里新增 `android/`，
  与 `web/` 平行。
- **设计改动从此有两处要改。** `docs/07-design-language.md` 仍是唯一事实来源，
  `web/src/styles/theme.css` 与 `android/app/src/main/java/.../ui/theme/Tokens.kt`
  是它的两份实现。改 token 要同时改两边 —— 这条负担是选 C 换来的，明写在这里。
- API 契约变化会同时打到两个客户端。`docs/api/openapi.yaml` 仍是唯一契约；
  web 用 `openapi-typescript` 生成，Android 侧的模型是手写的
  （`kotlinx.serialization`），所以**契约改了要跑一遍 `android` 的模型对齐检查**。
- 分发不走应用商店，走 hub 自己的 `/download`（[ADR-0010](0010-public-apk-download.md)）。

### 什么情况下重新审视

- 如果 iOS 也要做：那时 C 的复用率是零，值得重新掂量 Compose Multiplatform 或 RN。
  **现在只做 Android，不为一个还不存在的 iOS 端预付跨平台的复杂度。**
- 如果「几乎一致」在实践中被证明维护不动（两边 token 长期漂移），
  应该先做 token 的机器生成（从一份 JSON 同时吐 CSS 与 Kotlin），而不是退回 WebView。
