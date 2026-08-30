plugins {
    kotlin("jvm") version "2.0.21"
}

// app 通过 group:name 匹配替换成这个构建（Gradle 的 composite build 自动做）
group = "org.agenthub"
version = "0.1.0"

// 刻意**不用 jvmToolchain(17)**：那会要求机器上正好装着 JDK 17，
// 装了 21 也不算数，而且没配 toolchain 下载源时直接构建失败。
// 只钉编译目标（app 侧 AGP 要的是 17 字节码），在 17/21 上都能编。
java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}
kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

repositories {
    mavenCentral()
}

dependencies {
    testImplementation(kotlin("test"))
}

tasks.test {
    useJUnitPlatform()
    testLogging {
        events("passed", "failed", "skipped")
    }
}
