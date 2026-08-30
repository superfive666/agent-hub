pluginManagement {
    repositories {
        google {
            content {
                includeGroupByRegex("com\\.android.*")
                includeGroupByRegex("com\\.google.*")
                includeGroupByRegex("androidx.*")
            }
        }
        mavenCentral()
        gradlePluginPortal()
    }
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        google()
        mavenCentral()
    }
}

rootProject.name = "agent-hub-android"

// core 是一个**独立构建**，用 includeBuild 组合进来（不是 include(":core")）。
// 理由写在 core/settings.gradle.kts 里：这样它的单元测试在没有 Android SDK
// 的机器上也能跑。app 侧只要声明 implementation("org.agenthub:core")，
// Gradle 会按 group:name 自动替换成这个构建。
includeBuild("core")

include(":app")
