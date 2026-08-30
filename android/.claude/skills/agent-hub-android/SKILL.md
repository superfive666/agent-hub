---
name: agent-hub-android
description: 在 android/ 里写任何代码时用 —— 新页面、改设计、接契约、发版。它把「这个 app 只是第二个前端」这条前提、设计语言五条不变量在 Compose 里的落法、以及三个会静默咬人的技术坑交给你。写第一行 Kotlin 之前读。
---

# agent-hub Android 客户端

先读根 [CLAUDE.md](../../../CLAUDE.md) 的三条设计前提、
[立项书](../../../docs/09-android-app.md)、
[ADR-0009](../../../docs/adr/0009-android-native-compose.md)（为什么原生而不是套壳）、
[设计语言](../../../docs/07-design-language.md)。**ADR 是有约束力的。**

## 1. 一条前提

**这个 app 只是第二个前端。** 不存业务数据、不做本地决策、不新增任何后端 ——
打的是和 `web/` 完全相同的 `/api/admin/*`，用的是完全相同的会话 cookie。

从这条推出来的、不用再讨论的结论：

- **不加本地缓存。** 缓存一旦存在，「界面上看到的」和「hub 上真实的」就可能不一致，
  而这个平台的整个正确性模型建立在「以 hub 为准」上。
- **不碰 `/api/agent/*`。** 那套 Bearer 凭证是给 agent 的，app 是给那一个人类管理员的。
- **不为 app 改后端。** 想加端点之前先问：web 上这件事是怎么做的？
  唯一为 app 加过的是 `/download`，而它服务的是「怎么把 app 发出去」，不是运行时。

## 2. 五条不变量在这儿落在哪

设计语言 §1 那五条**是功能不是审美**，在这个工程里是硬指标：

| 不变量 | 落在哪 | 破了会怎样 |
|---|---|---|
| §1.1 人和 agent 一眼分得开（位置/气泡/`@`/chip 四重） | `ui/components/Bubble.kt` | 屏幕再窄也**不许**改成「都靠左、用颜色区分」—— 那正是这条在防的事 |
| §1.2 主 agent 与关注者的层级 | `ui/components/Avatar.kt` 的 `AvatarKind` | 关注者的虚线是**语义**：被 @ 只产生关注关系，没有回复义务 |
| §1.3 特效只给有语义的地方 | 辉光/呼吸只挂 `AvatarKind.Primary` | 每张头像都发光的话，辉光不再意味着任何东西 |
| §1.4 outbox 告警不可折叠 | `OutboxBanner.kt` + `AppScaffold` 放在**每一页**顶部 | worker 挂掉是完全静默的失败，这是唯一能发现它的地方 |
| §1.5 尊重「移除动画」 | `ui/theme/Theme.kt` 的 `LocalReduceMotion` | 新加的动画**都要问它**，不要绕过 |

窄屏下最先牺牲的是布局，**不是** §1 的任何一条。

## 3. 三个会静默咬人的坑

都不报错，都很难从症状反推到原因。

### 3.1 棱镜描边必须用 `saveLayer` 隔离

`BlendMode.DstOut` 作用在**当前图层里已经画下的一切**上。直接在
`drawWithContent { drawContent(); …DstOut… }` 里挖，挖掉的不是那圈渐变的中间，
而是**整块玻璃板加上它所有的内容** —— 屏幕中间一个跟着动的透明洞。

`ui/glass/Prism.kt` 里那两行 `canvas.saveLayer(...)` / `canvas.restore()`
**不要删，也不要"优化"成直接画**。改之前先看真机截图。

（web 那条 `transform: translateZ(0)` 是 Chromium 合成器特有的，这里不存在。）

### 3.2 模糊要 API 31+，降级路径**保证可读性不保证像**

低于 31 时玻璃板退回**不透明实底** `paneFallback`，
**不是**"糊一层半透明白" —— 半透明白压在舞台的深青蓝上会把深色文字吃掉，
那正是设计语言里记过的「亮色第一版失败」的原因。

**不要为了模糊抬 minSdk。** 那会把一批还在用的机器直接排除掉。

### 3.3 `Load` 取值只走 `valueOrNull()`

`as? Load.Ok` 会把类型实参擦成 `Load.Ok<*>`，`.value` 于是是 `Any?`。
新补一个页面时很容易重新踩，所以取值只有这一个入口。

## 4. 测试

**领域规则的测试全部写在 `core/`**，那是一个不依赖 Android 的独立 Gradle 构建。

```bash
./gradlew -p core test          # 不需要 Android SDK，几秒钟
./gradlew :app:testDebugUnitTest # 需要 SDK
```

和 Go 那边同一条纪律：**用例按需求写，不按实现写。**
新加一条规则时先问「需求文档/ADR 里哪一句话在说这件事」，把那句话变成用例名。

哪些该进 `core/`：状态流转、看板按平台时区切天、@ 提及解析、hub 地址规整、
outbox 何时必须出声。它们都有需求可依，而且都在没有模拟器的机器上测得了。

哪些留在 `app/`：会话 cookie 罐、HTTP 客户端（MockWebServer）。

**Composable 不写测试。** 界面的正确性靠 §2 那张表和真机截图，
写一堆 `assertNodeExists` 只会把重构成本抬上去而抓不到 §1 那类问题。

## 5. 改了 API 契约之后

`docs/api/openapi.yaml` 是唯一契约。web 那边用 `openapi-typescript` **生成**，
**这边的 `data/Dto.kt` 是手写的** —— 契约改了这边不会自动跟上。
这条负担明写在 ADR-0009 的「影响」里，处理方式是：改完契约把 `data/Dto.kt` 过一遍，
然后让 CI 把 app 编一遍（`.github/workflows/android.yml` 把 openapi.yaml 也放进了触发路径）。

每个 DTO 都给默认值、`ignoreUnknownKeys` 开着：hub 加字段时旧版 app 不该崩，
不开的话**每个响应都解析失败、界面全空**，而后端看起来一切正常。

## 6. 设计 token 有两份实现

`web/src/styles/theme.css` 和 `android/app/.../ui/theme/Tokens.kt`。
唯一事实来源是 `docs/07-design-language.md`。

**改 token 要同时改两边。** 这是选原生换来的负担，写在 ADR-0009 里。
真到了维护不动的那天，做法是让一份 JSON 同时吐 CSS 和 Kotlin，
**而不是退回 WebView 壳**。

## 7. 发版

APK **不进 git**（ADR-0010）。发版是打一个 `android-v<versionName>` 的 tag：
CI 构建并建 GitHub Release → 管理员把包放到 hub 的 `ANDROID_APK_PATH` →
控制台和 `/download` 就能下到。详见 [部署 §5.5](../../../docs/08-deployment.md)。

tag 上有三道闸，都会当场失败：没有签名 secret、tag 与 `versionName` 对不上、
产物是 debug 签名的。GitHub Release 只是归档 —— 对外的正式地址是 hub 的
`/download`，内网部署上 GitHub 根本不可达。

**签名密钥丢了 = 所有已安装用户必须卸载重装。** keystore 走 CI secret，
备份责任写在立项书的风险表里。
