# Android 客户端立项

**代号**：`android/`　**形态**：原生 Kotlin + Jetpack Compose 的单模块 App
**定位**：agent-hub 控制台的第二个前端。**它只是一个前端** —— 不存业务数据、
不做本地决策、不新增任何后端。

先读 [立项书](00-charter.md)、[设计语言](07-design-language.md)、
[ADR-0009](adr/0009-android-native-compose.md)（为什么是原生而不是套壳）、
[ADR-0010](adr/0010-public-apk-download.md)（怎么发出去）。

---

## 1. 为什么要有它

`web/` 已经适配了 H5，手机浏览器上能用。多做一个 app 的理由不是「网页不能用」，
而是这三件网页做不到的事：

| 场景 | 网页上现在怎样 | app 能怎样 |
|---|---|---|
| agent 回了话，人不在控制台前 | 完全不知道，得自己开页面看 | 系统通知（v2） |
| 想快速看一眼「谁卡住了」 | 打开浏览器 → 找标签页 → 等加载 | 桌面图标一点就在 |
| 地铁里、信号差 | 白屏，或者转圈到超时 | 上次拉到的内容还在（v2 离线缓存） |

v1 **不做**通知和离线缓存 —— 但地基（原生栈）必须现在选对，否则这两件事都要推倒重来。
这正是 [ADR-0009](adr/0009-android-native-compose.md) 拒绝 WebView 壳的理由。

## 2. 范围

### v1 做什么

**全量对齐 web 控制台的七个页面**，一个不少：

| 页面 | 对应 web 路由 | 关键点 |
|---|---|---|
| 登录 | `/login` | 口令 + Google OIDC 两种入口都摆出来，由实例自己拒绝不属于它的那种 |
| 对话 | `/threads/:id` | 设计语言 §1.1 的四重信号、§1.2 的主 agent/关注者层级、确认闸门 |
| 看板 | `/board` | 按天，`activity` / `started` 两种分组 |
| 待办 | `/todos` | 按状态筛选，含「待确认」那一档 |
| 名录 | `/directory` | Agent Card：能做什么、边界在哪 |
| 新建 todo | `/todos/new` | 主 agent 必选且唯一；@ 只产生关注者 |
| 名录 · 新建 agent | `/directory/new` | 建完给出一次性注册 token 与那句接入 prompt |
| 设置 | `/settings` | 含 **outbox 告警带**（§1.4：任何屏幕宽度下都不可折叠） |

比 web 多一件事：**hub 地址由用户自己填**。网页天然知道自己从哪来，
app 不知道 —— 首次启动要问「你的 hub 在哪」，这是 app 独有的第一屏。

### v1 不做什么

明确不做，不是"以后再说"：

- **不做系统通知 / 后台拉取**。要做对的话得先想清楚 [ADR-0006](adr/0006-gateway-outbox-no-sse.md)
  那套 inbox + cursor 在移动端怎么落（FCM？前台服务？轮询？），
  塞进 v1 只会得到一个耗电又漏消息的版本。
- **不做离线缓存**。同上：缓存一旦存在，「界面上看到的」和「hub 上真实的」
  就可能不一致，而这个平台的整个正确性模型建立在「以 hub 为准」上。
- **不做 agent 侧的接口**。app 是给**那一个人类管理员**用的，
  `/api/agent/*` 那套 Bearer 凭证不出现在 app 里。
- **不上应用商店**。见 [ADR-0010](adr/0010-public-apk-download.md)。

## 3. 和 hub 的关系

```
┌────────────┐                    ┌──────────────┐
│  android/  │ ── HTTPS ────────► │              │
│  (Compose) │   Cookie: hub_ses… │   agent-hub  │
└────────────┘                    │   (Go, 不变)  │
┌────────────┐                    │              │
│   web/     │ ── HTTPS ────────► │              │
└────────────┘   同一套 /api/admin └──────────────┘
```

**后端一行不改。** app 打的是和控制台完全相同的 `/api/admin/*`，
用的是完全相同的会话 —— hub 的会话是 `HttpOnly` Cookie（`hub_session`，
HMAC 签名，12 小时），app 用 OkHttp 的 `CookieJar` 存这一个 cookie，
行为与浏览器一致。**不新发 token，不新增鉴权模型。**

唯一为 app 加的端点是 [`/download`](adr/0010-public-apk-download.md)，
而它服务的是「怎么把 app 发出去」，不是 app 运行时要用的。

### 登录的两种模式

实例在部署时二选一，互斥。app 和 web 一样：**两种入口都摆出来**，
由实例自己拒绝不属于它的那一种（未登录时 `/api/admin/me` 是 401，
拿不到 `authMode`，所以客户端没法预先知道）。

- **口令**：`POST /api/admin/login`，与 web 完全一致。
- **Google OIDC**：走**应用内 WebView**，不是 Custom Tabs。
  Custom Tabs 的 cookie 落在浏览器进程里，app 拿不到；只有 WebView 的
  `CookieManager` 能把 `hub_session` 交给 OkHttp。这是这个方案唯一一处
  「app 和浏览器不一样」的地方，理由写在代码注释里。

## 4. 设计还原：判定标准

**不追像素级还原，追的是 [设计语言](07-design-language.md) §1 那五条不变量一条不少。**
它们是功能不是审美。§2 的构图（舞台 → 玻璃板 → 嵌套内板）和 §3 的厚玻璃
在 Compose 里重画，允许肉眼可辨的差异，**不允许语义上的差异**。

三处必须知道的技术差异：

| web 的做法 | Android 的做法 | 注意 |
|---|---|---|
| `backdrop-filter: blur(30px)` | `graphicsLayer { renderEffect = BlurEffect() }` | **需要 API 31+**；低版本降级成**不透明**的纯色面，不是半透明白 —— 半透明白压在舞台的深青蓝上会把深色文字吃掉，那正是亮色第一版失败的原因 |
| `conic-gradient` + `mask-composite: exclude` | `Brush.sweepGradient` + `BlendMode.DstOut` | web 那条 `translateZ(0)` 的坑是 Chromium 合成器特有的，Compose 没有 |
| `@media (prefers-reduced-motion)` | `Settings.Global.ANIMATOR_DURATION_SCALE == 0` | 系统「移除动画」开关，全局关掉动画 |

移动端的形态直接照 §4 的 `< 640px` 那一档：**底部 tab 取代侧栏、单列、无右栏，
右栏内容压成 thread 顶部状态带**。触控目标 ≥ 44dp，底部让出 `navigationBars` 安全区。
**不画假状态栏、不画假键盘** —— 系统会画在你的布局之上。

## 5. 模块划分

```
android/
  core/                     纯 Kotlin（无 Android 依赖）：领域模型、状态枚举、
                            格式化、时间/看板日期计算。**单元测试都在这儿**
  app/
    ui/theme/               Tokens.kt —— 语义变量，对应 web/src/styles/theme.css
    ui/glass/               Stage / Pane / Inset / Prism / Sheen —— 设计语言 §2/§3 的底盘
    ui/components/          Avatar / Bubble / Chip / Seg / Button / OutboxBanner
    ui/screens/             七个页面
    data/                   HubApi（OkHttp + kotlinx.serialization）、CookieJar、Session
    nav/                    底部 tab + 返回栈
```

**`core/` 是纯 JVM 模块**，这不是洁癖：它让「状态流转、看板按平台时区切天、
@ 提及解析」这些**有需求可依的规则**能在没有 Android SDK 的机器上测，
CI 里跑得飞快，也不会因为一个模拟器起不来就没人跑测试。

## 6. 里程碑

| # | 交付 | 完成的判据 |
|---|---|---|
| M0 | 立项、ADR、`/download` 端点、控制台下载入口 | 未登录状态下能从控制台点到 `/download`；没包时按钮是禁用加说明 |
| M1 | 工程骨架 + 设计底盘 + `core/` 及其测试 | 舞台/玻璃板/内板/棱镜边在真机上立得住；`core` 测试全绿 |
| M2 | 首屏（填 hub 地址）+ 登录（口令 + OIDC） | 两种模式都能拿到会话并保持 |
| M3 | 对话页 | §1.1 四重信号、§1.2 层级、确认闸门都在 |
| M4 | 看板 / 待办 / 名录 | 与 web 同数据同口径 |
| M5 | 新建 todo / 新建 agent / 设置（含 outbox 告警带） | §1.4 不可折叠 |
| M6 | 签名、CI 出包、发布流程跑通 | 从 tag 到 `/download` 能下到的包，全自动 |

## 7. 验收标准

M6 之后，这些必须成立：

1. **未登录也能下载 APK**，路径是 `/download`，`Content-Type` 是
   `application/vnd.android.package-archive`。
2. app 装上后，填入 hub 地址即可用**与控制台完全相同的账号**登录，
   不需要任何额外配置。
3. 设计语言 §1 的五条不变量在 app 上逐条成立（对着 §1 的表逐条过）。
4. `core/` 的单元测试覆盖需求文档里能落到客户端的那些规则
   （状态流转、看板按平台时区切天、@ 只产生关注者的展示层含义）。
5. **outbox 告警带在任何屏幕尺寸下都在首屏**，不可折叠、不可降级。
6. 系统开启「移除动画」时，app 里没有任何动画在跑。

## 8. 风险

| 风险 | 应对 |
|---|---|
| **两处 token 长期漂移** —— 设计改了只改了 web | 明写在 ADR-0009 的「影响」里；真的维护不动时做 token 的机器生成（一份 JSON 吐 CSS + Kotlin），**而不是退回 WebView** |
| API 31 以下没有 `BlurEffect` | 降级成不透明面并保证对比度。**降级路径保证可读性，不是保证像** |
| 自建分发需要用户允许「安装未知来源」 | 绕不开，`android/README.md` 里有给最终用户的说明 |
| 签名密钥丢失 = 所有已安装用户必须卸载重装 | keystore 不进 git，走 CI secret；密钥的备份责任写进部署文档 |
| OIDC 走 WebView 的安全观感 | 只对**自己实例的域名**开 WebView，且不注入任何 JS 桥；这一点在代码注释里说明 |
