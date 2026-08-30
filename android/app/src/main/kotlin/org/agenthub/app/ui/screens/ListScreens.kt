package org.agenthub.app.ui.screens

import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.rememberScrollState
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
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import org.agenthub.app.Load
import org.agenthub.app.valueOrNull
import org.agenthub.app.data.AgentSummary
import org.agenthub.app.data.TodoSummary
import org.agenthub.app.ui.components.Avatar
import org.agenthub.app.ui.components.AvatarKind
import org.agenthub.app.ui.components.ButtonTone
import org.agenthub.app.ui.components.Chip
import org.agenthub.app.ui.components.ChipTone
import org.agenthub.app.ui.components.PageHeader
import org.agenthub.app.ui.components.PillButton
import org.agenthub.app.ui.glass.Inset
import org.agenthub.app.ui.theme.tokens
import org.agenthub.core.STATUS_FLOW
import org.agenthub.core.TodoStatus
import org.agenthub.core.AgentStatus
import org.agenthub.core.agentStatusLabel
import org.agenthub.core.initialsOf
import org.agenthub.core.isAwaitingConfirmation
import org.agenthub.core.statusLabel
import org.agenthub.core.timeLabel
import org.agenthub.core.tierLabel
import java.time.ZoneId

/**
 * 取数三态的统一画法。
 *
 * **空列表和"还没拉到"要分开** —— 都画成空白的话，用户分不清是
 * 「这儿本来就没东西」还是「还在转」，而这两种情况该做的事完全不同。
 */
@Composable
fun <T> LoadBox(
    state: Load<List<T>>,
    emptyText: String,
    modifier: Modifier = Modifier,
    content: @Composable (List<T>) -> Unit,
) {
    val t = tokens()
    when (state) {
        is Load.Idle, is Load.Loading -> Box(modifier.fillMaxSize(), Alignment.Center) {
            Text("正在拉取…", color = t.ink3, style = MaterialTheme.typography.bodySmall)
        }
        is Load.Err -> Box(modifier.fillMaxSize().padding(24.dp), Alignment.Center) {
            Text(state.message, color = t.alert, style = MaterialTheme.typography.bodySmall)
        }
        is Load.Ok -> if (state.value.isEmpty()) {
            Box(modifier.fillMaxSize().padding(24.dp), Alignment.Center) {
                Text(emptyText, color = t.ink3, style = MaterialTheme.typography.bodySmall)
            }
        } else {
            content(state.value)
        }
    }
}

/** 对话列表。点进去是 thread 详情。 */
@Composable
fun ColumnScope.ThreadsScreen(
    todos: Load<List<TodoSummary>>,
    zone: ZoneId,
    onOpen: (String) -> Unit,
    onNewTodo: () -> Unit,
) {
    PageHeader(
        title = "对话",
        subtitle = "所有交互都经过 hub —— agent 之间没有直连",
    )
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 4.dp),
        horizontalArrangement = Arrangement.End,
    ) {
        PillButton(text = "新建 todo", tone = ButtonTone.Primary, onClick = onNewTodo)
    }
    Inset(modifier = Modifier.weight(1f).fillMaxWidth().padding(14.dp)) {
        LoadBox(todos, "还没有对话。建一条 todo，指一个主 agent，它就会开始说话。") { list ->
            LazyColumn(
                modifier = Modifier.fillMaxSize().padding(8.dp),
                verticalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                items(list, key = { it.threadId }) { todo ->
                    TodoRow(todo = todo, zone = zone, onClick = { onOpen(todo.threadId) })
                }
            }
        }
    }
}

/** 待办。比对话页多一个状态筛选器 —— 值域就是 STATUS_FLOW。 */
@Composable
fun ColumnScope.TodosScreen(
    todos: Load<List<TodoSummary>>,
    zone: ZoneId,
    onOpen: (String) -> Unit,
) {
    var filter by remember { mutableStateOf("all") }
    val all = todos.valueOrNull().orEmpty()
    // 「待确认」不是一个 status，是 confirmedAt 这一位（ADR-0008）。
    // 它必须能被筛出来 —— 卡在闸门上的 todo 是这个页面最该帮人找到的那一类。
    val awaitingConfirm = all.count { isAwaitingConfirmation(TodoStatus.from(it.status), it.confirmedAt) }

    PageHeader(
        title = "待办",
        subtitle = if (awaitingConfirm > 0) {
            "$awaitingConfirm 条在等你确认才能往下走"
        } else {
            "每条 todo 有且只有一个主 agent"
        },
    )

    // 筛选器用**横向滚动的 chip 排**而不是分段器：七档挤进一个分段器里，
    // 每格只剩两个字宽，中文会被截成半个词。横滚至少每一档都读得全。
    val options = buildList {
        add("all" to "全部")
        add("gate" to "待确认")
        STATUS_FLOW.forEach { add(it.wire to statusLabel(it)) }
    }
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .horizontalScroll(rememberScrollState())
            .padding(horizontal = 14.dp, vertical = 2.dp),
        horizontalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        options.forEach { (value, label) ->
            Box(modifier = Modifier.clickable { filter = value }) {
                Chip(
                    text = label,
                    tone = when {
                        filter != value -> ChipTone.Neutral
                        value == "gate" -> ChipTone.Warn
                        else -> ChipTone.Agent
                    },
                )
            }
        }
    }

    val shown = when (filter) {
        "all" -> all
        "gate" -> all.filter { isAwaitingConfirmation(TodoStatus.from(it.status), it.confirmedAt) }
        else -> all.filter { it.status == filter }
    }

    Inset(modifier = Modifier.weight(1f).fillMaxWidth().padding(14.dp)) {
        if (todos is Load.Ok && shown.isEmpty()) {
            Box(Modifier.fillMaxSize().padding(24.dp), Alignment.Center) {
                Text(
                    if (filter == "gate") "没有卡在确认闸门上的 todo。" else "这一档里没有 todo。",
                    color = tokens().ink3,
                    style = MaterialTheme.typography.bodySmall,
                )
            }
        } else {
            LoadBox(todos, "还没有 todo。") {
                LazyColumn(
                    modifier = Modifier.fillMaxSize().padding(8.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    items(shown, key = { it.threadId }) { todo ->
                        TodoRow(todo = todo, zone = zone, onClick = { onOpen(todo.threadId) })
                    }
                }
            }
        }
    }
}

@Composable
private fun TodoRow(todo: TodoSummary, zone: ZoneId, onClick: () -> Unit) {
    val t = tokens()
    val gated = isAwaitingConfirmation(TodoStatus.from(todo.status), todo.confirmedAt)
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick)
            .padding(horizontal = 10.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Avatar(
            initials = initialsOf(todo.primaryAgentName),
            // 列表里的头像画成 Primary：这一行就是那条 todo，
            // 而它的主 agent 只有一个。辉光在这里是有语义的。
            kind = AvatarKind.Primary,
            online = todo.primaryAgentOnline,
            label = "@${todo.primaryAgentName.orEmpty()}",
            size = 32.dp,
        )
        Column(modifier = Modifier.weight(1f)) {
            Text(
                todo.title,
                color = t.ink,
                style = MaterialTheme.typography.titleSmall,
                maxLines = 1,
            )
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                modifier = Modifier.padding(top = 4.dp),
            ) {
                Text(
                    "@${todo.primaryAgentName.orEmpty()} · ${statusLabel(todo.status)}",
                    color = t.ink3,
                    style = MaterialTheme.typography.bodySmall,
                    maxLines = 1,
                )
                // 卡在闸门上的必须一眼看出来 —— 否则人只会以为 agent 在偷懒
                if (gated) Chip("待确认", tone = ChipTone.Warn)
            }
        }
        Column(horizontalAlignment = Alignment.End) {
            Text(
                timeLabel(todo.updatedAt ?: todo.startedAt, zone),
                color = t.ink3,
                style = MaterialTheme.typography.labelSmall,
            )
            if (todo.replyCount > 0) {
                Chip("${todo.replyCount}", tone = ChipTone.Agent, modifier = Modifier.padding(top = 4.dp))
            }
        }
    }
}

/** 名录：谁在这儿、能做什么、边界在哪 —— 全部来自各自的 Agent Card。 */
@Composable
fun ColumnScope.DirectoryScreen(
    directory: Load<List<AgentSummary>>,
    notice: String?,
    onNewAgent: () -> Unit,
    onReissueToken: (String) -> Unit,
    onSetEnabled: (String, Boolean) -> Unit,
    onDelete: (String) -> Unit,
    onDismissNotice: () -> Unit,
) {
    PageHeader(title = "名录", subtitle = "谁在这儿、能做什么、边界在哪")
    Row(
        modifier = Modifier.fillMaxWidth().padding(horizontal = 14.dp, vertical = 4.dp),
        horizontalArrangement = Arrangement.End,
    ) {
        PillButton(text = "接入新 agent", tone = ButtonTone.Primary, onClick = onNewAgent)
    }
    if (notice != null) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 14.dp, vertical = 4.dp)
                .clickable { onDismissNotice() },
        ) {
            Chip(notice, tone = ChipTone.Warn)
        }
    }
    Inset(modifier = Modifier.weight(1f).fillMaxWidth().padding(14.dp)) {
        LoadBox(directory, "名录是空的。接入一个 agent 试试。") { list ->
            LazyColumn(
                modifier = Modifier.fillMaxSize().padding(8.dp),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                items(list, key = { it.agentId }) { a ->
                    AgentCardRow(
                        agent = a,
                        onReissueToken = { onReissueToken(a.agentId) },
                        onSetEnabled = { onSetEnabled(a.agentId, it) },
                        onDelete = { onDelete(a.agentId) },
                    )
                }
            }
        }
    }
}

@Composable
private fun AgentCardRow(
    agent: AgentSummary,
    onReissueToken: () -> Unit,
    onSetEnabled: (Boolean) -> Unit,
    onDelete: () -> Unit,
) {
    val t = tokens()
    var open by remember { mutableStateOf(false) }
    val disabled = agent.status == AgentStatus.Disabled.wire

    Column(
        modifier = Modifier
            .fillMaxWidth()
            .clickable { open = !open }
            .padding(10.dp),
    ) {
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Avatar(
                initials = initialsOf(agent.name),
                kind = AvatarKind.Agent,
                online = agent.online,
                label = "@${agent.name}",
                size = 32.dp,
            )
            Column(modifier = Modifier.weight(1f)) {
                Text("@${agent.name}", color = t.agentInk, style = MaterialTheme.typography.titleSmall)
                Text(
                    "${agentStatusLabel(agent.status)} · ${tierLabel(agent.tier)}",
                    color = t.ink3,
                    style = MaterialTheme.typography.labelSmall,
                )
            }
            // 「还没写 Agent Card」要能一眼看出来：没有 Card 的 agent 在名录上
            // 是一行空壳，不标出来的话看起来像它什么都不会。
            if (!agent.hasCard) Chip("未写 Card", tone = ChipTone.Warn)
        }

        if (!agent.summary.isNullOrBlank()) {
            Text(
                agent.summary,
                color = t.ink2,
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.padding(top = 8.dp),
            )
        }
        if (agent.skills.isNotEmpty()) LabelRow("会做", agent.skills, ChipTone.Agent)
        // **边界必须画出来。** Agent Card 里 limitations 是必填的（留空后端 422），
        // 画不出来等于把这条要求白白浪费掉。
        if (agent.limitations.isNotEmpty()) LabelRow("不做", agent.limitations, ChipTone.Neutral)

        if (open) {
            Row(
                modifier = Modifier.padding(top = 10.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                // token 用掉即作废、24 小时也会自己过期 —— 补签是常规操作，
                // 不是异常处理，所以摆在第一位。
                PillButton(text = "补签 token", tone = ButtonTone.Default, onClick = onReissueToken)
                PillButton(
                    text = if (disabled) "启用" else "停用",
                    tone = ButtonTone.Default,
                    onClick = { onSetEnabled(disabled) },
                )
                // 删除排在最后且是危险色：绝大多数情况下正确的动作是停用 ——
                // 有留痕的 agent 根本删不掉，那是「一条 todo 必须有主 agent」
                // 这条硬约束在说话。
                PillButton(text = "删除", tone = ButtonTone.Danger, onClick = onDelete)
            }
        }
    }
}

@Composable
private fun LabelRow(label: String, values: List<String>, tone: ChipTone) {
    val t = tokens()
    Column(modifier = Modifier.padding(top = 8.dp)) {
        Text(label, color = t.ink3, style = MaterialTheme.typography.labelSmall)
        Row(
            modifier = Modifier.padding(top = 4.dp),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            values.take(3).forEach { Chip(it, tone = tone) }
            if (values.size > 3) Chip("+${values.size - 3}", tone = ChipTone.Neutral)
        }
    }
}
