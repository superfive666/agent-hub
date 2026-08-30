# kotlinx.serialization 的序列化器是编译期生成的，R8 看不出谁在用它们。
# 少了这几条，release 包在解析任何一个响应时抛
# "Serializer for class 'X' is not found" —— 而 debug 包完全正常。
-keepattributes *Annotation*, InnerClasses
-dontnote kotlinx.serialization.**

-keepclassmembers class org.agenthub.app.data.** {
    *** Companion;
}
-keepclasseswithmembers class org.agenthub.app.data.** {
    kotlinx.serialization.KSerializer serializer(...);
}
-keep,includedescriptorclasses class org.agenthub.app.data.**$$serializer { *; }

# OkHttp 在 JVM 上引用了一些可选的平台类，R8 会对缺失的引用发警告
-dontwarn okhttp3.internal.platform.**
-dontwarn org.conscrypt.**
-dontwarn org.bouncycastle.**
-dontwarn org.openjsse.**
