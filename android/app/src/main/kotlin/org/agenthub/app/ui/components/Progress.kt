package org.agenthub.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.PathEffect
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.unit.dp
import org.agenthub.app.ui.theme.tokens
import org.agenthub.core.GATE_LABEL
import org.agenthub.core.ProgressStep
import org.agenthub.core.StepState

/**
 * 进度条。步骤由 core 的 `progressOf` 算，这里只负责画。
 *
 * **需求确认那一格长得不一样**：它不是一个 status，是 `confirmedAt` 这一位
 * 数据推出来的闸门（ADR-0008）。未确认的 todo 卡在这儿推不动，
 * 进度条上却完全看不出原因的话，人只会以为 agent 在偷懒 ——
 * 所以它用虚线圈画，一眼能看出「这一格和别的不是一类东西」。
 *
 * 手机上装不下五六格，所以**横向滚动**而不是压缩字号：
 * 压到 8sp 谁都读不出来，等于这条进度条不存在。
 */
@Composable
fun ProgressRail(steps: List<ProgressStep>, modifier: Modifier = Modifier) {
    val t = tokens()
    Row(
        modifier = modifier.horizontalScroll(rememberScrollState()),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        steps.forEachIndexed { i, step ->
            if (i > 0) {
                Box(
                    modifier = Modifier
                        .width(14.dp)
                        .height(1.dp)
                        .background(t.hair2),
                )
            }
            val gate = step.label == GATE_LABEL
            val dotColor = when (step.state) {
                StepState.Done -> t.agent
                StepState.Current -> t.human
                StepState.Todo -> Color.Transparent
            }
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(
                    modifier = Modifier
                        .size(10.dp)
                        .then(
                            if (step.state == StepState.Todo) {
                                Modifier.drawBehind {
                                    drawCircle(
                                        color = t.ink3.copy(alpha = 0.7f),
                                        style = Stroke(
                                            width = 1.2f,
                                            // 闸门那一格永远用虚线 —— 它和别的格子不是一类东西
                                            pathEffect = if (gate) {
                                                PathEffect.dashPathEffect(floatArrayOf(3f, 3f))
                                            } else {
                                                null
                                            },
                                        ),
                                    )
                                }
                            } else {
                                Modifier.background(dotColor, CircleShape)
                            },
                        )
                        .then(
                            if (gate && step.state != StepState.Todo) {
                                Modifier.border(1.dp, t.agentInk, CircleShape)
                            } else {
                                Modifier
                            },
                        ),
                )
                Text(
                    text = step.label,
                    color = when (step.state) {
                        StepState.Current -> t.ink
                        StepState.Done -> t.ink2
                        StepState.Todo -> t.ink3
                    },
                    style = MaterialTheme.typography.labelSmall,
                    modifier = Modifier.padding(start = 5.dp),
                )
            }
        }
    }
}
