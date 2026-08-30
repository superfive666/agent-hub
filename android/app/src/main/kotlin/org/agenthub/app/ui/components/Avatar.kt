package org.agenthub.app.ui.components

import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.scale
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.material3.Text
import org.agenthub.app.ui.theme.LocalReduceMotion
import org.agenthub.app.ui.theme.UiFont
import org.agenthub.app.ui.theme.tokens

/**
 * 头像的四种身份。**这不是四种配色，是四种语义**（设计语言 §1.1 / §1.2）。
 */
enum class AvatarKind {
    /** 那一个人类。暖橘渐变实底 —— 在一堆机器发言里必须立刻认得出来 */
    Human,

    /** 这条 todo 的主 agent。虹彩渐变 + 双层辉光 + 呼吸缩放 */
    Primary,

    /**
     * 关注者。**虚线描边、透明底、无辉光。**
     * 虚线在这里是语义：被 @ 只产生关注关系，没有回复义务 —— 它不是一个承诺。
     */
    Watcher,

    /** 普通 agent（名录里、不在某条 thread 语境下） */
    Agent,
}

/**
 * 头像。
 *
 * @param online 在线与否。**null = 不知道**，画成灰点而不是离线点 ——
 *   画成离线是在陈述一件我们并不知道的事。
 */
@Composable
fun Avatar(
    initials: String,
    kind: AvatarKind,
    modifier: Modifier = Modifier,
    size: Dp = 34.dp,
    online: Boolean? = null,
    label: String? = null,
) {
    val t = tokens()
    val reduceMotion = LocalReduceMotion.current

    // 呼吸缩放只给主 agent。§1.3：特效只给有语义的地方 ——
    // 每个头像都在呼吸的话，呼吸就不再意味着"这条 todo 归它管"。
    val breathe = if (kind == AvatarKind.Primary && !reduceMotion) {
        rememberInfiniteTransition(label = "breathe").animateFloat(
            initialValue = 1f,
            targetValue = 1.055f,
            animationSpec = infiniteRepeatable(
                animation = tween(4800, easing = FastOutSlowInEasing),
                repeatMode = RepeatMode.Reverse,
            ),
            label = "breathe-scale",
        ).value
    } else {
        1f
    }

    val fill: Brush = when (kind) {
        AvatarKind.Human -> t.meGrad
        AvatarKind.Primary -> Brush.linearGradient(listOf(t.i1, t.i2, t.i3, t.i5))
        AvatarKind.Watcher -> Brush.linearGradient(listOf(Color.Transparent, Color.Transparent))
        AvatarKind.Agent -> Brush.linearGradient(listOf(t.agentSoft, t.agentSoft))
    }
    val ink = when (kind) {
        AvatarKind.Human -> t.meInk
        AvatarKind.Primary -> if (t.dark) Color(0xFF10141A) else Color(0xFF04443A)
        else -> t.agentInk
    }

    Box(
        modifier = modifier
            .size(size)
            .scale(breathe)
            .then(
                if (kind == AvatarKind.Primary) {
                    // 双层辉光。**只有主 agent 有** —— 如果每张头像都在发光，
                    // 辉光就不再意味着任何东西。
                    Modifier.drawBehind {
                        val r = this.size.minDimension / 2f
                        drawCircle(
                            brush = Brush.radialGradient(
                                listOf(t.i5.copy(alpha = 0.55f), Color.Transparent),
                                radius = r * 2.1f,
                            ),
                            radius = r * 2.1f,
                        )
                        drawCircle(
                            brush = Brush.radialGradient(
                                listOf(t.i2.copy(alpha = 0.35f), Color.Transparent),
                                radius = r * 1.55f,
                            ),
                            radius = r * 1.55f,
                        )
                    }
                } else {
                    Modifier
                },
            )
            .clip(CircleShape)
            .background(fill, CircleShape)
            .then(
                when (kind) {
                    // 虚线描边 = 「没有回复义务」。实线会读成"也是负责人之一"。
                    AvatarKind.Watcher -> Modifier.drawBehind {
                        drawCircle(
                            color = t.agent.copy(alpha = 0.55f),
                            radius = this.size.minDimension / 2f - 1f,
                            style = Stroke(
                                width = 1.4f,
                                pathEffect = PathEffect.dashPathEffect(floatArrayOf(4f, 4f)),
                            ),
                        )
                    }
                    AvatarKind.Agent -> Modifier.border(1.dp, t.hair2, CircleShape)
                    else -> Modifier
                },
            )
            .semantics { if (label != null) contentDescription = label },
        contentAlignment = Alignment.Center,
    ) {
        Text(
            text = initials,
            color = if (kind == AvatarKind.Watcher) t.agentInk else ink,
            fontFamily = UiFont,
            fontWeight = FontWeight.ExtraBold,
            fontSize = (size.value * 0.34f).sp,
        )
        if (online != null) {
            OnlineDot(online = online, parentSize = size, modifier = Modifier.align(Alignment.BottomEnd))
        }
    }
}

@Composable
private fun OnlineDot(online: Boolean, parentSize: Dp, modifier: Modifier = Modifier) {
    val t = tokens()
    val d = (parentSize.value * 0.28f).dp.coerceAtLeast(8.dp)
    Box(
        modifier = modifier
            .size(d)
            .background(if (online) t.agent else t.ink3, CircleShape)
            .border(1.5.dp, if (t.dark) Color(0xFF10141A) else Color.White, CircleShape),
    )
}
