package org.agenthub.app.ui.theme

import android.provider.Settings
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.TextStyle

/** 用户在设置里选的主题。跟随系统是默认。 */
enum class ThemeMode { System, Light, Dark }

/**
 * 系统是不是关掉了动画。
 *
 * 设计语言 §1.5：动效必须尊重 `prefers-reduced-motion`。
 * Android 上对应的是「设置 → 无障碍 → 移除动画」，读
 * `ANIMATOR_DURATION_SCALE == 0`。
 *
 * **新增动画不要绕过它。** 呼吸、流光、高光扫过全部要问这一位；
 * 前庭功能敏感的人被一屏持续运动的界面影响是真实的生理反应，
 * 不是偏好设置。
 */
val LocalReduceMotion = staticCompositionLocalOf { false }

@Composable
fun rememberReduceMotion(): Boolean {
    val context = LocalContext.current
    return remember(context) {
        runCatching {
            Settings.Global.getFloat(
                context.contentResolver,
                Settings.Global.ANIMATOR_DURATION_SCALE,
                1f,
            ) == 0f
        }.getOrDefault(false)
    }
}

/**
 * Material 的 ColorScheme 在这套设计里**基本用不上** —— 颜色全走 [Tokens]。
 * 这里只给它一组不会打架的值，免得某个 Material 组件（涟漪、文本选择手柄）
 * 自己冒出一个紫色出来。
 */
private fun schemeOf(t: Tokens) = if (t.dark) {
    darkColorScheme(
        primary = t.agent, onPrimary = t.priInk, background = t.stage.last().second,
        surface = t.menuBg, onSurface = t.ink, error = t.alert,
    )
} else {
    lightColorScheme(
        primary = t.agent, onPrimary = t.priInk, background = t.stage.last().second,
        surface = t.menuBg, onSurface = t.ink, error = t.alert,
    )
}

@Composable
fun AgentHubTheme(
    mode: ThemeMode = ThemeMode.System,
    content: @Composable () -> Unit,
) {
    val dark = when (mode) {
        ThemeMode.System -> isSystemInDarkTheme()
        ThemeMode.Light -> false
        ThemeMode.Dark -> true
    }
    val tokens = if (dark) DarkTokens else LightTokens
    val reduceMotion = rememberReduceMotion()

    CompositionLocalProvider(
        LocalTokens provides tokens,
        LocalReduceMotion provides reduceMotion,
    ) {
        MaterialTheme(
            colorScheme = schemeOf(tokens),
            typography = AppTypography,
            content = content,
        )
    }
}

/** 取当前 token。写 `tokens()` 比 `LocalTokens.current` 短，用得太频繁了。 */
@Composable
fun tokens(): Tokens = LocalTokens.current

/** `.lbl` 那种小标签：全大写在中文里没意义，所以只保留字重和字距。 */
@Composable
fun labelStyle(): TextStyle = MaterialTheme.typography.labelSmall.copy(color = tokens().ink3)
