package org.agenthub.app.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
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
import org.agenthub.app.valueOrNull
import org.agenthub.app.data.ThreadDetail
import org.agenthub.app.data.TodoStep
import org.agenthub.app.ui.components.ButtonTone
import org.agenthub.app.ui.components.Bubble
import org.agenthub.app.ui.components.Chip
import org.agenthub.app.ui.components.ChipTone
import org.agenthub.app.ui.components.PageHeader
import org.agenthub.app.ui.components.PillButton
import org.agenthub.app.ui.components.PillField
import org.agenthub.app.ui.components.ProgressRail
import org.agenthub.app.ui.glass.Inset
import org.agenthub.app.ui.theme.tokens
import org.agenthub.core.AuthorKind
import org.agenthub.core.TodoStatus
import org.agenthub.core.authorOf
import org.agenthub.core.isAwaitingConfirmation
import org.agenthub.core.mentionedAgentIds
import org.agenthub.core.progressOf
import org.agenthub.core.stepKindLabel
import org.agenthub.core.stepStatusLabel
import org.agenthub.core.timeLabel
import java.time.ZoneId

/**
 * 一条 thread 的详情。
 *
 * 布局按设计语言 §4 的 `< 640px` 那一档：**单列、无右栏，右栏的内容
 * （状态、进度、处理步骤）压成顶部的状态带**。这不是"手机上简化了"，
 * 这就是那一档的规定形态。
 */
@Composable
fun ColumnScope.ThreadScreen(
    thread: Load<ThreadDetail>,
    steps: Load<List<TodoStep>>,
    directory: List<Pair<String, String>>,
    zone: ZoneId,
    onBack: () -> Unit,
    onReply: (String, List<String>) -> Unit,
    onConfirm: () -> Unit,
    onStatus: (String) -> Unit,
) {
    val t = tokens()
    val detail = thread.valueOrNull()

    PageHeader(
        title = detail?.title ?: "对话",
        subtitle = detail?.let { "@${it.primaryAgentName.orEmpty()}" },
    )
    Row(modifier = Modifier.padding(horizontal = 14.dp)) {
        PillButton(text = "← 返回", tone = ButtonTone.Ghost, onClick = onBack)
    }

    when (thread) {
        is Load.Err -> {
            Box(Modifier.fillMaxSize().padding(24.dp), Alignment.Center) {
                Text(thread.message, color = t.alert, style = MaterialTheme.typography.bodySmall)
            }
            return
        }
        is Load.Idle, is Load.Loading -> {
            Box(Modifier.fillMaxSize(), Alignment.Center) {
                Text("正在拉取…", color = t.ink3, style = MaterialTheme.typography.bodySmall)
            }
            return
        }
        is Load.Ok -> Unit
    }
    val d = detail ?: return

    val status = TodoStatus.from(d.status)
    val gated = isAwaitingConfirmation(status, d.confirmedAt)

    // ── 状态带：宽屏上的右栏在这一档里压成这一条 ──
    Column(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 6.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        ProgressRail(
            steps = progressOf(status, d.confirmedAt),
            modifier = Modifier.fillMaxWidth(),
        )
        if (gated) {
            // **闸门必须显式画出来。** 未确认的 todo 卡在这儿推不动，
            // 进度条上看不出原因的话，人只会以为 agent 在偷懒（ADR-0008）。
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .background(t.warnSoft, RoundedCornerShape(14.dp))
                    .padding(12.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                Text("需求确认", color = t.warn, style = MaterialTheme.typography.titleSmall)
                Text(
                    "这条 todo 在等你点头才能往下走。agent 现在能做的只有把需求问清楚 —— " +
                        "它不会自己放行，那是设计上的。",
                    color = t.warn,
                    style = MaterialTheme.typography.bodySmall,
                )
                PillButton(text = "确认放行", tone = ButtonTone.Primary, onClick = onConfirm)
            }
        }
        if (d.watchers.isNotEmpty()) {
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Text("关注", color = t.ink3, style = MaterialTheme.typography.labelSmall)
                d.watchers.take(4).forEach { w ->
                    // 被 @ 只产生关注关系，没有回复义务 —— 所以是「关注」这一档
                    Chip("@${w.name.orEmpty()}", tone = ChipTone.Watcher)
                }
            }
        }
        val stepList = steps.valueOrNull().orEmpty()
        if (stepList.isNotEmpty()) {
            StepsStrip(stepList)
        }
    }

    // ── 消息流 ──
    val listState = rememberLazyListState()
    LaunchedEffect(d.posts.size) {
        if (d.posts.isNotEmpty()) listState.animateScrollToItem(d.posts.lastIndex)
    }
    val watcherOnline = d.watchers.associate { it.agentId to (it.online ?: false) }

    Inset(
        modifier = Modifier
            .weight(1f)
            .fillMaxWidth()
            .padding(horizontal = 14.dp, vertical = 6.dp),
    ) {
        if (d.posts.isEmpty()) {
            Box(Modifier.fillMaxSize().padding(24.dp), Alignment.Center) {
                Text(
                    "还没有人说话。写一句，主 agent 会收到。",
                    color = t.ink3,
                    style = MaterialTheme.typography.bodySmall,
                )
            }
        } else {
            LazyColumn(
                state = listState,
                modifier = Modifier.fillMaxSize().padding(12.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp),
            ) {
                items(d.posts, key = { it.postId }) { post ->
                    Bubble(
                        author = authorOf(
                            authorKind = if (post.authorKind == "admin") AuthorKind.Admin else AuthorKind.Agent,
                            authorId = post.authorId,
                            authorName = post.authorName,
                            primaryAgentId = d.primaryAgentId,
                            watcherOnline = watcherOnline,
                        ),
                        body = post.body,
                        time = timeLabel(post.createdAt, zone),
                    )
                }
            }
        }
    }

    Composer(
        directory = directory,
        onSend = onReply,
        modifier = Modifier.fillMaxWidth().padding(horizontal = 14.dp).padding(bottom = 12.dp),
    )

    // 状态推进。放在输入框之后：日常动作是「说话」，改状态是偶发的。
    if (status != null && status != TodoStatus.Done && status != TodoStatus.Cancelled) {
        Row(
            modifier = Modifier.fillMaxWidth().padding(horizontal = 14.dp).padding(bottom = 10.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            PillButton(
                text = "标记完成",
                tone = ButtonTone.Default,
                onClick = { onStatus(TodoStatus.Done.wire) },
            )
            PillButton(
                text = "取消",
                tone = ButtonTone.Danger,
                onClick = { onStatus(TodoStatus.Cancelled.wire) },
            )
        }
    }
}

/** 处理步骤。post 是「说了什么」，step 是「做到哪一步了」—— 两回事。 */
@Composable
private fun StepsStrip(steps: List<TodoStep>) {
    val t = tokens()
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text("处理详情", color = t.ink3, style = MaterialTheme.typography.labelSmall)
        steps.takeLast(3).forEach { s ->
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                Chip(stepKindLabel(s.kind), tone = ChipTone.Agent)
                Text(
                    s.title,
                    color = t.ink2,
                    style = MaterialTheme.typography.bodySmall,
                    maxLines = 1,
                    modifier = Modifier.weight(1f),
                )
                Text(stepStatusLabel(s.status), color = t.ink3, style = MaterialTheme.typography.labelSmall)
            }
        }
    }
}

/**
 * 回复框。
 *
 * **底色是不透明的** `composerBg`，不是内板那套很淡的半透明 —— 它压在会滚动的
 * 消息流上方，透太多的话滚过去的字会从底下浮上来，和输入的字叠在一起，
 * 两边都读不清。这条在 web 上记过一次，这里同样成立。
 */
@Composable
private fun Composer(
    directory: List<Pair<String, String>>,
    onSend: (String, List<String>) -> Unit,
    modifier: Modifier = Modifier,
) {
    val t = tokens()
    var text by remember { mutableStateOf("") }
    Row(
        modifier = modifier
            .background(t.composerBg, RoundedCornerShape(24.dp))
            .imePadding()
            .padding(8.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        PillField(
            value = text,
            onValueChange = { text = it },
            placeholder = "说点什么，@ 谁就通知谁",
            singleLine = false,
            modifier = Modifier.weight(1f),
        )
        PillButton(
            text = "发送",
            tone = ButtonTone.Primary,
            enabled = text.isNotBlank(),
            onClick = {
                // @ 解析在 core 里（有单测）。收边写错的代价是发通知给一个
                // 无关的 agent，而发出去就收不回来。
                onSend(text, mentionedAgentIds(text, directory))
                text = ""
            },
        )
    }
}
