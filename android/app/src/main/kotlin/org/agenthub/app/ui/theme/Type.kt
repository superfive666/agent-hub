package org.agenthub.app.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp
import org.agenthub.app.R

/**
 * 字体。
 *
 * - **Manrope**：UI 与全部正文。圆润、字腔开放 —— **不是 Inter**，
 *   那是两种完全不同的气质，换掉之后整套的"软"就没了。
 * - **JetBrains Mono**：只用于真正的机器内容（agent id、seq、token、
 *   runtime 名、代码）。**不拿它当风格用** —— 等宽字在这里是「这串东西
 *   是给机器看的」这个语义，滥用之后就不再意味着任何东西。
 *
 * ## 为什么字体文件是打进包里的，不走 Google Fonts 运行时下载
 *
 * 第一版用的是 `androidx.compose.ui.text.googlefonts`，被两件事挡回来：
 *
 * 1. 那条路要一份 Google Play 服务的签名证书清单。它**不在 androidx.core 里**
 *    （`androidx.core.R.array.com_google_android_gms_fonts_certs` 是不存在的），
 *    要么自己把 Google 的证书哈希抄进仓库，要么另引一个依赖。
 * 2. 更要紧的是**它依赖 Play 服务**。这个平台是自建的，用户里有一部分机器
 *    根本没有 Play 服务 —— 那时字体静默落回系统 sans，整套排版的字形记号就没了，
 *    而且没有任何地方会报错。
 *
 * 打进包里换来的是：确定的字形、离线可用、不依赖任何外部服务。
 * 代价是约 600 KB 的 APK 体积，对一个十几 MB 的包是划算的。
 *
 * 中文由系统栈接手（PingFang SC / Noto Sans SC）—— Manrope 本来就没有汉字，
 * 这一点和 web 上完全一致。
 *
 * 字体是 SIL Open Font License 1.1，见 `android/FONTS-LICENSE.txt`。
 *
 * ⚠️ `res/font/` 下**只能放字体文件** —— AAPT2 会把这个目录里的每个文件
 * 都当字体去编译，放个 .txt 进去 build 直接失败。
 */
val UiFont = FontFamily(
    Font(R.font.manrope_medium, FontWeight.Medium),
    Font(R.font.manrope_semibold, FontWeight.SemiBold),
    Font(R.font.manrope_bold, FontWeight.Bold),
    Font(R.font.manrope_extrabold, FontWeight.ExtraBold),
)

/** 只给机器内容用。见上面那段说明。 */
val MonoFont = FontFamily(
    Font(R.font.jetbrains_mono_medium, FontWeight.Medium),
    Font(R.font.jetbrains_mono_bold, FontWeight.Bold),
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
