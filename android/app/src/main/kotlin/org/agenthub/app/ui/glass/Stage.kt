package org.agenthub.app.ui.glass

import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import org.agenthub.app.ui.theme.LocalReduceMotion
import org.agenthub.app.ui.theme.tokens
import kotlin.math.cos
import kotlin.math.sin

/**
 * 舞台 —— 设计语言 §2 的第一层。
 *
 * **这是底盘，不是装饰。** 玻璃板浮在这上面才有质感：亮色下它是
 * 深青蓝 → 冰白的纵向落差，把背景做成均匀的高键淡彩，厚度感就没了。
 *
 * 面板之间留缝、四周留白，**让舞台背景透出来** —— 悬浮感全靠这些缝，
 * 贴边就退回普通页面底色了。
 *
 * @param flat 内容直接坐在舞台上时（全屏文档页）用不带深色段的那版 ——
 *   深青蓝那一段是给「板浮在上面」准备的，不是给正文准备的。
 */
@Composable
fun Stage(
    modifier: Modifier = Modifier,
    flat: Boolean = false,
    content: @Composable BoxScope.() -> Unit,
) {
    val t = tokens()
    val reduceMotion = LocalReduceMotion.current
    val stops = (if (flat) t.stageFlat else t.stage).toTypedArray()

    // 三团色雾缓慢移动，30 秒一圈。§1.5：系统关了动画就完全静止 ——
    // 不是变慢，是不动。
    val phase = if (reduceMotion) {
        0f
    } else {
        val transition = rememberInfiniteTransition(label = "stage")
        transition.animateFloat(
            initialValue = 0f,
            targetValue = 1f,
            animationSpec = infiniteRepeatable(
                animation = tween(30_000, easing = LinearEasing),
                repeatMode = RepeatMode.Restart,
            ),
            label = "stage-phase",
        ).value
    }

    Box(
        modifier = modifier
            .fillMaxSize()
            .drawBehind {
                // 展开运算符不能省：colorStops 是 vararg，直接传数组会挑到 List<Color> 那个重载
                drawRect(Brush.verticalGradient(*stops))

                // 色雾。半径按短边取，横竖屏都不会糊成一片。
                val r = minOf(size.width, size.height) * 0.85f
                t.stageAura.forEachIndexed { i, color ->
                    val a = (phase + i * 0.37f) * 2f * Math.PI.toFloat()
                    val cx = size.width * (0.2f + 0.6f * (0.5f + 0.5f * cos(a)))
                    val cy = size.height * (0.1f + 0.5f * (0.5f + 0.5f * sin(a * 0.7f)))
                    drawCircle(
                        brush = Brush.radialGradient(
                            colors = listOf(color, Color.Transparent),
                            center = Offset(cx, cy),
                            radius = r,
                        ),
                        radius = r,
                        center = Offset(cx, cy),
                    )
                }
            },
        content = content,
    )
}

/**
 * 舞台四周留白。**背景要看得见才叫舞台**，贴边就退回普通页面底色了。
 * web 上是 38px，手机屏窄，收到 14dp —— 但**不能到 0**。
 */
val StagePadding = 14.dp

/** 面板之间的缝。少了它两块板会粘成一块，悬浮感全无。 */
val PaneGap = 12.dp
