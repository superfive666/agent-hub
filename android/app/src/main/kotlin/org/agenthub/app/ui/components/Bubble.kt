package org.agenthub.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.unit.dp
import org.agenthub.app.ui.theme.tokens
import org.agenthub.core.AuthorView
import org.agenthub.core.Participation

/**
 * 一条发言 —— **设计语言 §1.1 的四重信号在这里全部落地**。
 *
 * | 信号 | 人类 | Agent |
 * |---|---|---|
 * | 位置 | 靠右 | 靠左 |
 * | 气泡 | 暖橘渐变实底 + 辉光 | 玻璃面 |
 * | 名字 | 不带前缀 | 带 `@` |
 * | 标签 | 永远挂「人类」chip | 无 |
 *
 * **少一重都不行**，而且不能只靠颜色 —— 色弱、灰度截图、缩到手机屏都得成立。
 * **位置是最强的那一重**：任何布局改动都不能把人和 agent 混在同一列里。
 *
 * 屏幕再窄也不许改成"都靠左、用颜色区分" —— 那正是这条不变量在防的事。
 */
@Composable
fun Bubble(
    author: AuthorView,
    body: String,
    time: String,
    modifier: Modifier = Modifier,
) {
    val t = tokens()
    val human = author.isHuman

    Row(
        modifier = modifier.fillMaxWidth(),
        // 信号一：位置。人靠右，agent 靠左。
        horizontalArrangement = if (human) Arrangement.End else Arrangement.Start,
        verticalAlignment = Alignment.Top,
    ) {
        if (!human) {
            Avatar(
                initials = author.initials,
                kind = if (author.participation == Participation.Primary) {
                    AvatarKind.Primary
                } else {
                    AvatarKind.Watcher
                },
                online = author.online,
                label = author.displayName,
                modifier = Modifier.padding(end = 9.dp, top = 2.dp),
            )
        }

        Column(
            horizontalAlignment = if (human) Alignment.End else Alignment.Start,
            modifier = Modifier.widthIn(max = 300.dp),
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                // 信号三：名字。agent 一律带 @，人类不带。
                Text(
                    text = author.displayName,
                    color = if (human) t.humanInk else t.agentInk,
                    style = MaterialTheme.typography.labelMedium,
                )
                // 信号四：标签。人类**永远**挂「人类」chip，一次都不省。
                when {
                    human -> Chip("人类", tone = ChipTone.Human)
                    author.participation == Participation.Primary -> Chip("主 agent", tone = ChipTone.Agent)
                    else -> Chip("关注", tone = ChipTone.Watcher)
                }
                Text(time, color = t.ink3, style = MaterialTheme.typography.labelSmall)
            }

            val shape = RoundedCornerShape(
                topStart = if (human) 18.dp else 6.dp,
                topEnd = if (human) 6.dp else 18.dp,
                bottomStart = 18.dp,
                bottomEnd = 18.dp,
            )
            // 信号二：气泡。人是暖橘渐变实底，agent 是玻璃面。
            val fill: Brush = if (human) t.meGrad else SolidColor(t.chipBg)
            Column(
                modifier = Modifier
                    .padding(top = 6.dp)
                    .clip(shape)
                    .background(fill, shape)
                    .then(
                        when {
                            human -> Modifier
                            // 主 agent 的气泡带一圈青色描边微光。§1.2 的层级
                            author.participation == Participation.Primary ->
                                Modifier.border(1.dp, t.agent.copy(alpha = 0.45f), shape)
                            else -> Modifier.border(1.dp, t.hair2, shape)
                        },
                    )
                    .padding(horizontal = 14.dp, vertical = 11.dp),
            ) {
                Text(
                    text = body,
                    color = if (human) t.meInk else t.ink,
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }

        if (human) {
            Avatar(
                initials = author.initials,
                kind = AvatarKind.Human,
                label = author.name,
                modifier = Modifier.padding(start = 9.dp, top = 2.dp),
            )
        }
    }
}
