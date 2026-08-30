package org.agenthub.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import org.agenthub.app.ui.theme.tokens
import org.agenthub.core.OutboxState
import org.agenthub.core.outboxMessage

/**
 * outbox 告警带 —— **设计语言 §1.4，这条不可折叠、不可降级、不可挪到二级页面。**
 *
 * worker 挂掉是**完全静默**的失败：帖子照发、inbox 照拉，只是没有新东西。
 * `outbox_lag` 是唯一能发现它的地方。所以这条带子挂在**每一个**页面的顶部，
 * 不随页面走、不做成"可以收起来的通知"、不因为"太吵"降级。
 *
 * 会想动它的理由总是"手机屏这么小，它占了一行"。那一行正是它的全部价值 ——
 * 屏幕小到装不下这条告警时，该牺牲的是别的东西（§4：窄屏下最先牺牲的是布局，
 * **不是** §1 的任何一条）。
 *
 * `liveRegion` 是给读屏用的：状态从 Quiet 变成告警时要主动播报，
 * 而不是等用户自己摸到这一行。
 */
@Composable
fun OutboxBanner(state: OutboxState, modifier: Modifier = Modifier) {
    if (state is OutboxState.Quiet) return
    val t = tokens()

    // Unknown 和 WorkerDown 是同一档严重程度：前者是"探针也没了"，
    // 后者是"确定坏了"。都用 alert 色；Lagging 是 warn。
    val severe = state !is OutboxState.Lagging
    val fg = if (severe) t.alert else t.warn
    val bg = if (severe) t.alertSoft else t.warnSoft
    val shape = RoundedCornerShape(16.dp)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .background(bg, shape)
            .border(1.dp, fg.copy(alpha = 0.4f), shape)
            .padding(horizontal = 14.dp, vertical = 11.dp)
            .semantics { liveRegion = LiveRegionMode.Assertive },
    ) {
        Row {
            Text(
                text = when (state) {
                    is OutboxState.WorkerDown -> "投递 worker 不在了"
                    OutboxState.Unknown -> "拿不到 outbox 状态"
                    is OutboxState.Lagging -> "事件投递滞后"
                    OutboxState.Quiet -> ""
                },
                color = fg,
                style = MaterialTheme.typography.titleSmall,
            )
        }
        Text(
            text = outboxMessage(state),
            color = fg,
            style = MaterialTheme.typography.bodySmall,
            modifier = Modifier.padding(top = 4.dp),
        )
    }
}
