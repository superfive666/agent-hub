package org.agenthub.app.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.googlefonts.GoogleFont
import androidx.compose.ui.unit.sp

/**
 * 字体。
 *
 * - **Manrope**：UI 与全部正文。圆润、字腔开放 —— **不是 Inter**，
 *   那是两种完全不同的气质，换掉之后整套的"软"就没了。
 * - **JetBrains Mono**：只用于真正的机器内容（agent id、seq、token、
 *   runtime 名、代码）。**不拿它当风格用** —— 等宽字在这里是"这串东西
 *   是给机器看的"这个语义，滥用之后就不再意味着任何东西。
 *
 * 走 Google Fonts 的运行时下载，所以字体文件**不进仓库**（省掉 ~200 KB，
 * 也免掉字重不全时的假粗体）。拿不到时（没有 Play 服务、离线首启）
 * 自动落回系统默认字体 —— 那时中文栈由系统给（PingFang / Noto Sans SC），
 * 拉丁字母会变成系统 sans。**这是可接受的降级：字形变了，排版不变。**
 */
private val provider = GoogleFont.Provider(
    providerAuthority = "com.google.android.gms.fonts",
    providerPackage = "com.google.android.gms",
    // **写全限定名**：gradle.properties 里开了 android.nonTransitiveRClass，
    // 本模块的 R 不再包含依赖库的资源，写成 R.array.… 会直接编不过。
    // 这份证书清单由 androidx.core 提供，不用自己抄那几串 base64。
    certificates = androidx.core.R.array.com_google_android_gms_fonts_certs,
)

private val manrope = GoogleFont("Manrope")
private val jetbrainsMono = GoogleFont("JetBrains Mono")

val UiFont = FontFamily(
    Font(googleFont = manrope, fontProvider = provider, weight = FontWeight.Medium),
    Font(googleFont = manrope, fontProvider = provider, weight = FontWeight.SemiBold),
    Font(googleFont = manrope, fontProvider = provider, weight = FontWeight.Bold),
    Font(googleFont = manrope, fontProvider = provider, weight = FontWeight.ExtraBold),
)

/** 只给机器内容用。见上面那段说明。 */
val MonoFont = FontFamily(
    Font(googleFont = jetbrainsMono, fontProvider = provider, weight = FontWeight.Medium),
    Font(googleFont = jetbrainsMono, fontProvider = provider, weight = FontWeight.Bold),
)

/**
 * 字号偏小、字重偏重、字距收紧 —— 这是这套设计的排版指纹，
 * 照抄 web 上那些 `text-[12.5px] font-bold tracking-[-0.03em]`。
 * 用 Material 的默认排版会立刻"变成另一个 app"。
 */
val AppTypography = Typography(
    displaySmall = TextStyle(
        fontFamily = UiFont, fontWeight = FontWeight.ExtraBold,
        fontSize = 25.sp, lineHeight = 34.sp, letterSpacing = (-0.9).sp,
    ),
    headlineSmall = TextStyle(
        fontFamily = UiFont, fontWeight = FontWeight.ExtraBold,
        fontSize = 19.sp, lineHeight = 24.sp, letterSpacing = (-0.6).sp,
    ),
    titleMedium = TextStyle(
        fontFamily = UiFont, fontWeight = FontWeight.ExtraBold,
        fontSize = 15.5f.sp, lineHeight = 20.sp, letterSpacing = (-0.45).sp,
    ),
    titleSmall = TextStyle(
        fontFamily = UiFont, fontWeight = FontWeight.Bold,
        fontSize = 13.5f.sp, lineHeight = 18.sp, letterSpacing = (-0.3).sp,
    ),
    bodyMedium = TextStyle(
        fontFamily = UiFont, fontWeight = FontWeight.Medium,
        fontSize = 13.sp, lineHeight = 21.sp,
    ),
    bodySmall = TextStyle(
        fontFamily = UiFont, fontWeight = FontWeight.Medium,
        fontSize = 11.5f.sp, lineHeight = 19.sp,
    ),
    labelMedium = TextStyle(
        fontFamily = UiFont, fontWeight = FontWeight.Bold,
        fontSize = 12.sp, lineHeight = 16.sp, letterSpacing = (-0.2).sp,
    ),
    /** 小标签：全大写 + 大字距，就是 web 里的 `.lbl` */
    labelSmall = TextStyle(
        fontFamily = UiFont, fontWeight = FontWeight.ExtraBold,
        fontSize = 10.sp, lineHeight = 12.sp, letterSpacing = 1.0.sp,
    ),
)

/** 机器内容的样式。agent id、seq、token 用它。 */
val MonoStyle = TextStyle(
    fontFamily = MonoFont, fontWeight = FontWeight.Medium,
    fontSize = 11.sp, lineHeight = 16.sp,
)
