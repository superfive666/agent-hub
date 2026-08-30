package org.agenthub.app.ui.glass

import android.os.Build
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import org.agenthub.app.ui.theme.LocalReduceMotion
import org.agenthub.app.ui.theme.tokens

/** 玻璃板圆角。web 上是 30px。 */
val PaneRadius = 26.dp

/** 嵌套内板圆角。**比外板小** —— 板中有板靠的就是这个差。 */
val InsetRadius = 20.dp

/** 卡片圆角。 */
val CardRadius = 18.dp

/**
 * 厚玻璃板 —— 设计语言 §2 的第二层、§3 的质感。
 *
 * 半透明加一条 1px 描边只能做出「一张纸」。厚玻璃要把光在玻璃里的行为
 * 拆开画。web 上是六层 box-shadow，这里对应地画：
 *
 * | web 的那一层 | 这里 |
 * |---|---|
 * | `inset 0 1.5px 0 #fff` 顶边高光 | 顶部一条 hair 色的亮线 —— **厚度感的第一来源** |
 * | `inset 0 22px 44px -28px #fff` 内顶泛光 | 顶部向下的白色渐变 |
 * | `inset 0 -30px 52px -34px` 内底暗部 | 底部向上的冷色渐变 |
 * | 两层外投影 | `Modifier.shadow` |
 *
 * 棱镜边由 [prism] 提供，它内部用 `saveLayer` 隔离 —— 那一层的注释里写了
 * 为什么不能省，改之前先读。
 *
 * ## 模糊有一条硬线：API 31
 *
 * `RenderEffect.createBlurEffect` 是 API 31 才有的。低于 31 时**降级成
 * 不透明的实底**（`paneFallback`），而不是"糊一层半透明白" ——
 * 半透明白压在舞台的深青蓝上会把深色文字吃掉，那正是设计语言里记过的
 * 「亮色第一版失败」的原因。**降级路径保证可读性，不是保证像。**
 *
 * ## 性能
 *
 * `backdrop-filter` 很贵，Android 上同样。同屏同时模糊的层控制在 ~8 个以内。
 * **内板和 card 不要再各自开一层模糊** —— 它们靠低透明度背景和描边就够了。
 */
@Composable
fun Pane(
    modifier: Modifier = Modifier,
    radius: Dp = PaneRadius,
    /** 棱镜边。列表里成百上千个小卡片就别开了，太贵 */
    prism: Boolean = true,
    /** 高光扫过。只给大板子，卡片上不要 */
    sheen: Boolean = true,
    content: @Composable BoxScope.() -> Unit,
) {
    val t = tokens()
    val shape = RoundedCornerShape(radius)
    val blurSupported = Build.VERSION.SDK_INT >= Build.VERSION_CODES.S

    var m = modifier
        .shadow(
            elevation = 26.dp,
            shape = shape,
            clip = false,
            ambientColor = Color(0xFF08324A),
            spotColor = Color(0xFF08324A),
        )
        .clip(shape)

    m = if (blurSupported) {
        // 真·毛玻璃：半透明渐变 + 背后那层由 Stage 提供的背景透上来。
        // Compose 没有等价于 backdrop-filter 的 API，最接近的是让这层
        // 足够透，靠舞台的色雾本身提供"糊过的背景"的观感。
        m.background(t.paneBg, shape)
    } else {
        // API < 31：实底。可读性优先。
        m.background(t.paneFallback, shape)
    }

    m = m
        .drawBehind {
            val r = radius.toPx()
            // 内顶泛光：光漫进玻璃内部
            drawRoundRect(
                brush = Brush.verticalGradient(
                    0f to t.hair,
                    0.18f to Color.Transparent,
                    startY = 0f,
                    endY = size.height,
                ),
                cornerRadius = CornerRadius(r, r),
            )
            // 内底暗部：玻璃厚度投在内部的影
            drawRoundRect(
                brush = Brush.verticalGradient(
                    0.72f to Color.Transparent,
                    1f to if (t.dark) Color(0x1A000000) else Color(0x143C7396),
                    startY = 0f,
                    endY = size.height,
                ),
                cornerRadius = CornerRadius(r, r),
            )
            // 顶边高光 —— 厚度感的第一来源，少了它整块板会塌成一张纸
            drawLine(
                color = t.hair,
                start = Offset(r * 0.6f, 0.75f),
                end = Offset(size.width - r * 0.6f, 0.75f),
                strokeWidth = 1.5f,
            )
        }

    if (sheen) m = m.sheen()
    val withPrism = if (prism) m.prism(radius) else m

    Box(modifier = withPrism, content = content)
}

/**
 * 高光扫过 —— 「液态」的来源。一条白带 14 秒横扫一次。
 *
 * web 上靠 `mix-blend-mode: overlay`。这里用一条低不透明度的白色渐变直接叠，
 * 观感接近，且不用为一个装饰层开混合模式的离屏合成。
 */
@Composable
fun Modifier.sheen(): Modifier {
    val t = tokens()
    val reduceMotion = LocalReduceMotion.current
    if (reduceMotion) return this // §1.5：关了动画就一动不动

    val x = rememberInfiniteTransition(label = "sheen").animateFloat(
        initialValue = -0.4f,
        targetValue = 1.4f,
        animationSpec = infiniteRepeatable(
            animation = tween(14_000, easing = LinearEasing),
            repeatMode = RepeatMode.Restart,
        ),
        label = "sheen-x",
    ).value

    return this.drawWithContent {
        drawContent()
        val band = size.width * 0.35f
        val cx = size.width * x
        drawRect(
            brush = Brush.linearGradient(
                colors = listOf(Color.Transparent, Color.White, Color.Transparent),
                start = Offset(cx - band, 0f),
                end = Offset(cx + band, size.height),
            ),
            alpha = t.sheenOpacity * 0.22f,
            size = Size(size.width, size.height),
        )
    }
}

/**
 * 嵌套内板 —— §2 的第三层。**板中有板是关键一步。**
 *
 * 它不是叠在外板上的另一张卡，是**嵌进去的**：更淡、更小圆角，
 * 靠内阴影的方向差表达凹陷。少了这一层，整个构图就塌回普通卡片列表。
 *
 * 刻意**不开模糊、不开棱镜边**：性能预算要留给两块玻璃板和可视区内的气泡。
 */
@Composable
fun Inset(
    modifier: Modifier = Modifier,
    radius: Dp = InsetRadius,
    content: @Composable BoxScope.() -> Unit,
) {
    val t = tokens()
    val shape = RoundedCornerShape(radius)
    Box(
        modifier = modifier
            .clip(shape)
            .background(t.insetBg, shape)
            .drawBehind {
                val r = radius.toPx()
                // 顶部内阴影 —— 「凹进去」全靠它。方向和 Pane 的顶边高光相反。
                drawRoundRect(
                    brush = Brush.verticalGradient(
                        0f to if (t.dark) Color(0x14000000) else Color(0x0F3C7396),
                        0.12f to Color.Transparent,
                        startY = 0f,
                        endY = size.height,
                    ),
                    cornerRadius = CornerRadius(r, r),
                )
                // 底边回光
                drawLine(
                    color = t.hair,
                    start = Offset(r * 0.6f, size.height - 0.75f),
                    end = Offset(size.width - r * 0.6f, size.height - 0.75f),
                    strokeWidth = 1f,
                )
            },
        content = content,
    )
}
