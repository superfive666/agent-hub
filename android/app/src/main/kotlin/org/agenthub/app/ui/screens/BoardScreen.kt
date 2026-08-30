package org.agenthub.app.ui.screens

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import org.agenthub.app.Load
import org.agenthub.app.data.BoardResponse
import org.agenthub.app.ui.components.ButtonTone
import org.agenthub.app.ui.components.Chip
import org.agenthub.app.ui.components.ChipTone
import org.agenthub.app.ui.components.PageHeader
import org.agenthub.app.ui.components.PillButton
import org.agenthub.app.ui.components.Seg
import org.agenthub.app.ui.glass.Inset
import org.agenthub.app.ui.theme.tokens
import org.agenthub.core.isToday
import org.agenthub.core.isoDate
import org.agenthub.core.shiftDate
import org.agenthub.core.statusLabel
import org.agenthub.core.timeLabel
import java.time.Instant
import java.time.ZoneId

/**
 * 看板：按天回看「这一天发生了什么」。
 *
 * ## 日期切分按**平台时区**，不是设备时区
 *
 * 这是 app 相对网页新增的一整类风险 —— 手机的时区是用户带着走的。
 * 用设备时区切天的话，同一条 todo 在新加坡和在伦敦会落到不同的格子里，
 * 而两个人看着同一块看板会争论谁记错了。
 *
 * 时区从 `/api/admin/me` 的 `timezone` 来（部署时定死的 `PLATFORM_TIMEZONE`），
 * 计算全在 core 的 `isoDate` / `shiftDate` 里，有单测。
 *
 * ## 两种分组不是"视图选项"，是两个问题
 *
 * - `activity`：这一天**发生了什么** —— 一条 thread 会跨多天反复出现
 * - `started`：这一天**开了哪些事、现在怎么样了** —— 每条只出现一次
 */
@Composable
fun ColumnScope.BoardScreen(
    board: Load<BoardResponse>,
    zone: ZoneId,
    onLoad: (date: String, groupBy: String) -> Unit,
    onOpen: (String) -> Unit,
) {
    val t = tokens()
    var date by remember(zone) { mutableStateOf(isoDate(Instant.now(), zone)) }
    var groupBy by remember { mutableStateOf("activity") }

    LaunchedEffect(date, groupBy) { onLoad(date, groupBy) }

    PageHeader(
        title = "看板",
        subtitle = "按天回看 · 日期按平台时区切分",
    )

    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        PillButton(text = "‹", tone = ButtonTone.Default, onClick = { date = shiftDate(date, -1) })
        Column(modifier = Modifier.weight(1f), horizontalAlignment = Alignment.CenterHorizontally) {
            Text(date, color = t.ink, style = MaterialTheme.typography.titleSmall)
            if (isToday(date, zone)) {
                Text("今天", color = t.agentInk, style = MaterialTheme.typography.labelSmall)
            }
        }
        // 未来的日期没有意义 —— 挡住它，比让人翻到明天看到空白要好
        PillButton(
            text = "›",
            tone = ButtonTone.Default,
            enabled = !isToday(date, zone),
            onClick = { date = shiftDate(date, 1) },
        )
    }

    Seg(
        options = listOf("activity" to "发生了什么", "started" to "开了哪些事"),
        value = groupBy,
        onValueChange = { groupBy = it },
        modifier = Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 8.dp),
    )

    Inset(modifier = Modifier.weight(1f).fillMaxWidth().padding(14.dp)) {
        when (board) {
            is Load.Err -> Box(Modifier.fillMaxSize().padding(24.dp), Alignment.Center) {
                Text(board.message, color = t.alert, style = MaterialTheme.typography.bodySmall)
            }
            is Load.Idle, is Load.Loading -> Box(Modifier.fillMaxSize(), Alignment.Center) {
                Text("正在拉取…", color = t.ink3, style = MaterialTheme.typography.bodySmall)
            }
            is Load.Ok -> {
                val items = board.value.items
                if (items.isEmpty()) {
                    Box(Modifier.fillMaxSize().padding(24.dp), Alignment.Center) {
                        Text("这一天没有动静。", color = t.ink3, style = MaterialTheme.typography.bodySmall)
                    }
                } else {
                    LazyColumn(
                        modifier = Modifier.fillMaxSize().padding(8.dp),
                        verticalArrangement = Arrangement.spacedBy(6.dp),
                    ) {
                        items(items, key = { it.threadId + (it.at ?: "") }) { item ->
                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .clickable { onOpen(item.threadId) }
                                    .padding(10.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(8.dp),
                            ) {
                                Column(modifier = Modifier.weight(1f)) {
                                    Text(
                                        item.title,
                                        color = t.ink,
                                        style = MaterialTheme.typography.titleSmall,
                                        maxLines = 1,
                                    )
                                    Text(
                                        "@${item.primaryAgentName.orEmpty()} · ${statusLabel(item.status)}" +
                                            if (item.posts > 0) " · ${item.posts} 条" else "",
                                        color = t.ink3,
                                        style = MaterialTheme.typography.bodySmall,
                                        maxLines = 1,
                                    )
                                }
                                // tweet 和 todo 要分得开：tweet 没有主责人也没有完成状态
                                if (item.kind == "tweet") Chip("广播", tone = ChipTone.Neutral)
                                Text(
                                    timeLabel(item.at, zone),
                                    color = t.ink3,
                                    style = MaterialTheme.typography.labelSmall,
                                )
                            }
                        }
                    }
                }
            }
        }
    }
}
