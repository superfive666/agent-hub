package org.agenthub.app.ui.glass

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.BlendMode
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Paint
import androidx.compose.ui.graphics.drawscope.drawIntoCanvas
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import org.agenthub.app.ui.theme.LocalReduceMotion
import org.agenthub.app.ui.theme.tokens

/**
 * 棱镜色散描边 —— 设计语言 §3.1。
 *
 * 做法和 web 上等价，但技术路径完全不同：
 *
 * | web | 这里 |
 * |---|---|
 * | `conic-gradient` 铺满 | `Brush.sweepGradient` 铺满 |
 * | `mask-composite: exclude` 掏空中间 | `saveLayer` + `BlendMode.DstOut` 挖掉中间 |
 * | 动画改注册过的 `@property --ang` | 旋转 sweep 的取色相位 |
 *
 * ## 这里有一个和 web 那条 `translateZ(0)` 同级别的坑
 *
 * web 那条是 Chromium 合成器特有的，Compose 没有。但 Compose 有自己的：
 *
 * **`DstOut` 会作用在「当前图层里已经画下的一切」上。** 直接在
 * `drawWithContent { drawContent(); …DstOut… }` 里挖，挖掉的不是那圈渐变的
 * 中间部分，而是**整块玻璃板加上它所有的内容** —— 屏幕中间出现一个跟着动的
 * 透明洞，而且不报任何错。
 *
 * `saveLayer` 就是解药：它开一个只装这圈渐变的临时图层，`DstOut` 只能挖到
 * 这一层里的东西，`restore()` 时再把剩下的那圈合成回去。
 * **这两行不要删，也不要"优化"成直接画。** 改之前先看真机截图。
 *
 * 亮色 `.48` 是玻璃边缘的色散；暗色提到 `.85` 就成了霓虹角度渐变描边 ——
 * 同一套技术，两种气候。
 *
 * 做成 `@Composable Modifier` 扩展而不是 Modifier.Node，是因为它要读 token、
 * 读「系统是否关了动画」、还要挂一个无限动画 —— 这三样只有在组合里拿得到。
 */
@Composable
fun Modifier.prism(
    cornerRadius: Dp,
    width: Dp = 1.4f.dp,
    animated: Boolean = true,
): Modifier {
    val t = tokens()
    val reduceMotion = LocalReduceMotion.current
    // §1.5：系统关了动画就固定在一个相位上，不是转慢
    val phase = if (animated && !reduceMotion) {
        rememberInfiniteTransition(label = "prism").animateFloat(
            initialValue = 0f,
            targetValue = 1f,
            animationSpec = infiniteRepeatable(
                animation = tween(9_000, easing = LinearEasing),
                repeatMode = RepeatMode.Restart,
            ),
            label = "prism-angle",
        ).value
    } else {
        0.12f // 静止时也要停在一个好看的相位，不是 0
    }

    val colors = listOf(t.i1, t.i2, t.i3, t.i4, t.i5, t.i1)
    val opacity = t.prismOpacity

    return this.drawWithContent {
        drawContent()
        val r = cornerRadius.toPx()
        val w = width.toPx()
        if (size.width <= 2 * w || size.height <= 2 * w) return@drawWithContent

        drawIntoCanvas { canvas ->
            // ↓ 这一层只装下面那圈渐变。DstOut 因此挖不到玻璃板和内容。
            canvas.saveLayer(Rect(Offset.Zero, size), Paint())
            drawRoundRect(
                brush = Brush.sweepGradient(rotateColors(colors, phase), center = center),
                cornerRadius = CornerRadius(r, r),
                alpha = opacity,
            )
            drawRoundRect(
                color = Color.Black,
                topLeft = Offset(w, w),
                size = Size(size.width - 2 * w, size.height - 2 * w),
                cornerRadius = CornerRadius((r - w).coerceAtLeast(0f), (r - w).coerceAtLeast(0f)),
                blendMode = BlendMode.DstOut,
            )
            canvas.restore()
        }
    }
}

/**
 * 把颜色环按相位旋转。
 *
 * 首尾同色，所以旋转之后 sweep 的接缝仍然连续 —— 少了这一条，
 * 会有一道固定的硬边卡在 0° 上跟着面板转。
 */
private fun rotateColors(colors: List<Color>, phase: Float): List<Color> {
    if (colors.size < 3) return colors
    val ring = colors.dropLast(1)
    val n = ring.size
    val shift = (((phase % 1f) + 1f) % 1f * n).toInt().coerceIn(0, n - 1)
    val rotated = ring.drop(shift) + ring.take(shift)
    return rotated + rotated.first()
}
