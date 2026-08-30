plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

android {
    namespace = "org.agenthub.app"
    compileSdk = 35

    defaultConfig {
        applicationId = "org.agenthub.app"
        // minSdk 26 有两个具体理由，不是拍的：
        //   1) java.time 从 API 26 起是平台自带的，不用上 desugaring ——
        //      而看板要按平台时区切天，那套日期逻辑全在 java.time 上。
        //   2) 自适应图标（adaptive icon）也是 26 起。
        // 玻璃模糊要 API 31+，低于 31 的走不透明降级（见 ui/glass/Pane.kt），
        // **不为它抬 minSdk** —— 那会把一批还在用的机器直接排除掉。
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.1.0"

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        vectorDrawables.useSupportLibrary = true
    }

    signingConfigs {
        create("release") {
            // keystore **不进 git**。CI 从 secret 解出来放到这些路径/环境变量里；
            // 本地不配也能 assembleRelease —— 只是会退回 debug 签名，
            // 装得上但不能覆盖安装正式包。别让"本地构建不了"成为常态。
            val storePath = System.getenv("ANDROID_KEYSTORE_PATH")
            if (!storePath.isNullOrBlank() && file(storePath).exists()) {
                storeFile = file(storePath)
                storePassword = System.getenv("ANDROID_KEYSTORE_PASSWORD")
                keyAlias = System.getenv("ANDROID_KEY_ALIAS")
                keyPassword = System.getenv("ANDROID_KEY_PASSWORD")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            val rel = signingConfigs.getByName("release")
            signingConfig = if (rel.storeFile != null) rel else signingConfigs.getByName("debug")
        }
        debug {
            applicationIdSuffix = ".debug"
            versionNameSuffix = "-debug"
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlin {
        compilerOptions {
            jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
        }
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    sourceSets["main"].java.srcDirs("src/main/kotlin")
    sourceSets["test"].java.srcDirs("src/test/kotlin")

    packaging {
        resources.excludes += setOf("/META-INF/{AL2.0,LGPL2.1}")
    }

    testOptions {
        unitTests.isReturnDefaultValues = true
    }
}

// 发版时 CI 要拿到这两个值。**版本号的唯一来源是上面的 defaultConfig**，
// tag 只是指向它的一个记号 —— workflow 会校验 `android-v<versionName>` 和
// 实际 tag 一致，对不上就停下来，不会发出一个自己都说不清是哪一版的包。
//
// 为什么是一个任务而不是让 CI 去 grep 这个文件：正则会在有人给那行加注释、
// 换个引号写法的那天**静默匹配不到**，然后产物名里的版本就空了。
// 任务打印的是 Gradle 自己读到的值。
val appVersionName = android.defaultConfig.versionName
val appVersionCode = android.defaultConfig.versionCode
tasks.register("printVersion") {
    description = "打印 versionName / versionCode，给发版流水线用"
    doLast {
        println("versionName=$appVersionName")
        println("versionCode=$appVersionCode")
    }
}

dependencies {
    implementation(libs.agenthub.core)

    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.navigation.compose)
    implementation(libs.androidx.datastore.preferences)

    implementation(platform(libs.compose.bom))
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.graphics)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.foundation)
    implementation(libs.compose.material3)
    implementation(libs.compose.material.icons)

    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)
    implementation(libs.kotlinx.coroutines.android)

    debugImplementation(libs.compose.ui.tooling)
    debugImplementation(libs.okhttp.logging)

    testImplementation(libs.junit)
    testImplementation(libs.kotlin.test)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)

    androidTestImplementation(platform(libs.compose.bom))
    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(libs.espresso.core)
    androidTestImplementation(libs.compose.ui.test.junit4)
    debugImplementation(libs.compose.ui.test.manifest)
}
