# agent-hub Android 客户端

agent-hub 控制台的第二个前端。**它只是一个前端**：不存业务数据、不做本地决策、
不新增任何后端 —— 打的是和 `web/` 完全相同的 `/api/admin/*`，用的是完全相同的会话。

先读：
- [立项书](../docs/09-android-app.md) —— 范围、里程碑、验收标准
- [ADR-0009](../docs/adr/0009-android-native-compose.md) —— 为什么是原生 Compose 而不是 WebView 壳
- [ADR-0010](../docs/adr/0010-public-apk-download.md) —— APK 怎么发出去
- [设计语言](../docs/07-design-language.md) —— **改任何界面之前先读它的 §1 五条不变量**

---

## 给最终用户：怎么装

1. 在控制台上点「下载 APK」，或者直接打开 `https://你的hub/download`。
   **不需要登录** —— 装 app 的时候你手上还没有会话，而你很可能正是想在手机上
   登录才来装的。
2. 手机会问要不要允许「安装未知来源的应用」。这是自建分发绕不开的一步：
   这个包没有走应用商店，所以系统不认识它的来源。
3. 装完打开，第一屏问你的 hub 地址 —— 就是你平时打开控制台的那一个。
   从浏览器地址栏整条复制过来也行，多余的路径会自动去掉。
4. 用**和控制台完全一样的账号**登录。

> 这个 app 不含任何实例数据，同一份包可以发给任何一台 hub 的用户。

---

## 工程结构

```
android/
  core/         纯 Kotlin，无 Android 依赖。领域模型、状态流转、看板日期、
                @ 提及解析、hub 地址规整。**单元测试都在这儿**
  app/          Compose 应用
    ui/theme/     Tokens.kt —— 对应 web/src/styles/theme.css
    ui/glass/     Stage / Pane / Inset / Prism —— 设计语言 §2/§3 的底盘
    ui/components/  Avatar / Bubble / Chip / 控件 / outbox 告警带
    ui/screens/   八个页面
    data/         OkHttp + kotlinx.serialization + 会话 cookie 罐
    nav/          底部 tab + 返回栈
```

### `core/` 为什么是一个**独立的 Gradle 构建**

`android/settings.gradle.kts` 用 `includeBuild("core")` 而不是 `include(":core")`。
只有一个理由，但足够充分：

> 它让状态流转、看板按平台时区切天、@ 提及解析这些**有需求可依的规则**，
> 在**没有 Android SDK 的机器上**也能测。

放在同一个构建里的话，Gradle 配置阶段会先去配 `:app`，而 AGP 没有 SDK 直接失败 ——
纯逻辑的测试也跟着跑不起来。CI 里那台跑单测的机器、以及任何一个只想改文案的人，
都不该为此装 4 GB 的 SDK。

---

## ⚠️ 当前状态：`app` 模块还没被编译过

立项时的开发环境访问不到 `dl.google.com`（Android SDK 与 AGP 插件都在那儿），
所以 `:app` 连 Gradle 配置阶段都进不去。

- `core/` 的 48 个用例**已经跑过、全绿** —— 这正是把它拆成不依赖 Android 的
  独立构建的回报。
- `app/` 做过跨文件引用和泛型的静态核对，**但编译错误一定还有**。
  第一次 `./gradlew :app:assembleDebug`（本地或 CI）会给出那份清单。

修完编译之前，界面的观感、玻璃质感、棱镜边这些都**没有被真机验证过**。

---

## 开发

```bash
# 纯逻辑测试。**不需要 Android SDK**，几秒钟跑完
./gradlew -p core test

# 需要 Android SDK（ANDROID_HOME 或 local.properties 里的 sdk.dir）
./gradlew :app:testDebugUnitTest      # app 层单测（cookie 罐、API 客户端）
./gradlew :app:assembleDebug          # 调试包
./gradlew :app:lint
```

`local.properties`（不进 git）：

```properties
sdk.dir=/path/to/Android/sdk
```

### 改界面之前

设计语言 [§1 的五条不变量](../docs/07-design-language.md#1-五条不变量改设计时不能破)
在这里是**硬指标**，不是审美：

| 不变量 | 在这个工程里落在哪 |
|---|---|
| §1.1 人和 agent 一眼分得开（四重信号） | `ui/components/Bubble.kt` |
| §1.2 主 agent 与关注者的层级 | `ui/components/Avatar.kt` 的 `AvatarKind` |
| §1.3 特效只给有语义的地方 | 辉光/呼吸只在 `AvatarKind.Primary`，流光只在当前会话 |
| §1.4 outbox 告警不可折叠 | `ui/components/OutboxBanner.kt` + `AppScaffold` 把它放在每一页顶部 |
| §1.5 尊重「移除动画」 | `ui/theme/Theme.kt` 的 `LocalReduceMotion`，每个动画都问它 |

有三处技术差异值得先知道：

- **玻璃模糊要 API 31+**。低于 31 降级成**不透明实底**（`paneFallback`），
  不是"糊一层半透明白" —— 半透明白压在舞台的深青蓝上会把深色文字吃掉。
  降级路径保证可读性，不是保证像。
- **棱镜描边靠 `saveLayer` 隔离**。直接用 `BlendMode.DstOut` 会挖穿整块玻璃板
  （屏幕中间一个跟着动的透明洞，不报任何错）。`ui/glass/Prism.kt` 里写了细节。
- **字体走 Google Fonts 运行时下载**，仓库里没有字体文件。拿不到时落回系统字体，
  中文由系统栈接手 —— 字形变了，排版不变。

### 改了 API 契约之后

`docs/api/openapi.yaml` 是唯一契约。web 那边用 `openapi-typescript` 生成，
**这边的 `data/Dto.kt` 是手写的** —— 所以契约改了要手动跟一遍。
这条负担明写在 ADR-0009 的「影响」里。

---

## 发版

```bash
# CI 里做，需要签名密钥（走 secret，不进 git）
ANDROID_KEYSTORE_PATH=... ANDROID_KEYSTORE_PASSWORD=... \
ANDROID_KEY_ALIAS=... ANDROID_KEY_PASSWORD=... \
  ./gradlew :app:assembleRelease
```

产物放到 hub 的 `ANDROID_APK_PATH` 指向的路径，改 `ANDROID_APK_VERSION`，
重启 api 就生效 —— 详见 [部署 §5.5](../docs/08-deployment.md)。

**APK 不进 git。** 签名密钥丢了的话，所有已安装的用户必须卸载重装才能升级 ——
备份责任写在立项书的风险表里。
