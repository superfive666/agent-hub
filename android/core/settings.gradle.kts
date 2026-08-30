// core 是一个**独立的 Gradle 构建**，由 android/settings.gradle.kts 用
// includeBuild 组合进来。这么做只有一个理由，但足够充分：
//
//   它让「状态流转、看板按平台时区切天、@ 提及解析」这些有需求可依的规则
//   **在没有 Android SDK 的机器上也能测**。
//
// 放在同一个构建里的话，Gradle 配置阶段会先去配 :app，而 AGP 没有 SDK
// 直接 fail —— 于是纯逻辑的测试也跟着跑不起来。CI 里那台跑单测的机器、
// 以及任何一个只想改文案的人，都不该为此装 4 GB 的 SDK。
rootProject.name = "core"
