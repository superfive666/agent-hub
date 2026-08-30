package org.agenthub.app.ui.screens

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import org.agenthub.app.data.AgentSummary
import org.agenthub.app.data.CreateTodoRequest
import org.agenthub.app.data.CreatedAgent
import org.agenthub.app.ui.components.Avatar
import org.agenthub.app.ui.components.AvatarKind
import org.agenthub.app.ui.components.ButtonTone
import org.agenthub.app.ui.components.Chip
import org.agenthub.app.ui.components.ChipTone
import org.agenthub.app.ui.components.PageHeader
import org.agenthub.app.ui.components.PillButton
import org.agenthub.app.ui.components.PillField
import org.agenthub.app.ui.components.Separator
import org.agenthub.app.ui.glass.Inset
import org.agenthub.app.ui.theme.MonoStyle
import org.agenthub.app.ui.theme.tokens
import org.agenthub.core.initialsOf
import org.agenthub.core.mentionedAgentIds

/**
 * 新建 todo。
 *
 * **主 agent 必选且唯一** —— 这条规则在数据库层就是 `primary_agent_id NOT NULL`，
 * 界面上的落法是：没选主 agent 时提交按钮**禁用**，而不是提交之后弹错。
 * 让人填完一整个表单再告诉他缺一项，是在浪费他刚才那两分钟。
 *
 * **@ 只产生关注者。** 正文里 @ 到的人拿到 `todo.mentioned`，
 * 但**不进任何人的工作队列** —— 被 @ 不是一个承诺。所以这一页上
 * 「主 agent」和「@ 到的人」是两个截然不同的控件，不能合成一个多选。
 */
@Composable
fun ColumnScope.NewTodoScreen(
    directory: List<AgentSummary>,
    onBack: () -> Unit,
    onCreate: (CreateTodoRequest) -> Unit,
) {
    val t = tokens()
    var title by remember { mutableStateOf("") }
    var body by remember { mutableStateOf("") }
    var primary by remember { mutableStateOf<String?>(null) }

    PageHeader(title = "新建 todo", subtitle = "一条 todo 有且只有一个主 agent")
    Row(modifier = Modifier.padding(horizontal = 14.dp)) {
        PillButton(text = "← 返回", tone = ButtonTone.Ghost, onClick = onBack)
    }

    Inset(
        modifier = Modifier
            .weight(1f)
            .fillMaxWidth()
            .padding(14.dp),
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .imePadding()
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text("标题", color = t.ink3, style = MaterialTheme.typography.labelSmall)
            PillField(value = title, onValueChange = { title = it }, modifier = Modifier.fillMaxWidth())

            Text("说明", color = t.ink3, style = MaterialTheme.typography.labelSmall)
            PillField(
                value = body,
                onValueChange = { body = it },
                placeholder = "把要做的事说清楚。@ 谁就把谁拉进来关注。",
                singleLine = false,
                shape = RoundedCornerShape(20.dp),
                modifier = Modifier.fillMaxWidth(),
            )

            Separator()

            Text("主 agent（必选）", color = t.ink3, style = MaterialTheme.typography.labelSmall)
            Text(
                "它是这条 todo 的唯一负责人。被 @ 到的人只是关注者，没有回复义务。",
                color = t.ink3,
                style = MaterialTheme.typography.bodySmall,
            )
            directory.forEach { a ->
                val selected = a.agentId == primary
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .clickable { primary = a.agentId }
                        .padding(vertical = 8.dp),
                    horizontalArrangement = Arrangement.spacedBy(10.dp),
                ) {
                    Avatar(
                        initials = initialsOf(a.name),
                        // 选中的那个画成 Primary：辉光在这里正是「它就是负责人」
                        kind = if (selected) AvatarKind.Primary else AvatarKind.Agent,
                        online = a.online,
                        label = "@${a.name}",
                        size = 30.dp,
                    )
                    Column(modifier = Modifier.weight(1f)) {
                        Text("@${a.name}", color = t.ink, style = MaterialTheme.typography.titleSmall)
                        if (!a.summary.isNullOrBlank()) {
                            Text(
                                a.summary,
                                color = t.ink3,
                                style = MaterialTheme.typography.bodySmall,
                                maxLines = 1,
                            )
                        }
                    }
                    if (selected) Chip("主 agent", tone = ChipTone.Agent)
                }
            }

            val mentions = mentionedAgentIds(body, directory.map { it.agentId to it.name })
            if (mentions.isNotEmpty()) {
                Separator()
                Text("正文里 @ 到的（只会成为关注者）", color = t.ink3, style = MaterialTheme.typography.labelSmall)
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    mentions.forEach { id ->
                        val name = directory.firstOrNull { it.agentId == id }?.name.orEmpty()
                        Chip("@$name", tone = ChipTone.Watcher)
                    }
                }
            }

            PillButton(
                text = "创建",
                tone = ButtonTone.Primary,
                // 没选主 agent 就禁用。提交之后再报错等于浪费用户刚才那两分钟。
                enabled = primary != null && title.isNotBlank(),
                modifier = Modifier.fillMaxWidth(),
                onClick = {
                    onCreate(
                        CreateTodoRequest(
                            title = title.trim(),
                            body = body,
                            primaryAgentId = primary!!,
                            mentions = mentions,
                        ),
                    )
                },
            )
        }
    }
}

/**
 * 接入一个新 agent。
 *
 * 建完拿到的**明文注册 token 只出现这一次** —— 关掉就再也拿不到了
 * （要补签得回名录里重新签一张）。所以这一页在拿到 token 之后会切换成
 * 一个只讲「把这句话发给那台机器」的页面，而不是把 token 混在表单里显示。
 */
@Composable
fun ColumnScope.NewAgentScreen(
    created: CreatedAgent?,
    onBack: () -> Unit,
    onCreate: (String, String?) -> Unit,
    onDone: () -> Unit,
    onCopy: (String) -> Unit,
) {
    val t = tokens()
    var name by remember { mutableStateOf("") }
    var summary by remember { mutableStateOf("") }

    PageHeader(
        title = if (created == null) "接入新 agent" else "把这句话发过去",
        subtitle = if (created == null) "像注册 CI runner 那样：拿一串一次性 token 过去跑一下" else null,
    )
    Row(modifier = Modifier.padding(horizontal = 14.dp)) {
        PillButton(
            text = if (created == null) "← 返回" else "完成",
            tone = ButtonTone.Ghost,
            onClick = { if (created == null) onBack() else onDone() },
        )
    }

    Inset(modifier = Modifier.weight(1f).fillMaxWidth().padding(14.dp)) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .imePadding()
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            if (created == null) {
                Text("名字", color = t.ink3, style = MaterialTheme.typography.labelSmall)
                Text(
                    "名字是 @ 提及的唯一标识，**建完不能改** —— 改掉会让历史正文里的 @旧名字静默失效。",
                    color = t.ink3,
                    style = MaterialTheme.typography.bodySmall,
                )
                PillField(value = name, onValueChange = { name = it }, modifier = Modifier.fillMaxWidth())

                Text("一句话简介", color = t.ink3, style = MaterialTheme.typography.labelSmall)
                PillField(
                    value = summary,
                    onValueChange = { summary = it },
                    placeholder = "它是干什么的",
                    modifier = Modifier.fillMaxWidth(),
                )
                PillButton(
                    text = "创建并签一张注册 token",
                    tone = ButtonTone.Primary,
                    enabled = name.isNotBlank(),
                    modifier = Modifier.fillMaxWidth(),
                    onClick = { onCreate(name.trim(), summary.trim().ifBlank { null }) },
                )
            } else {
                Chip("token 只显示这一次，关掉就得重新签", tone = ChipTone.Warn)
                Text(
                    "把下面这句话原样发给那台机器上的 agent，它会自己读完整个接入流程。",
                    color = t.ink2,
                    style = MaterialTheme.typography.bodySmall,
                )
                val prompt = created.joinUrl?.let {
                    "Join agent-hub: read $it and follow it end to end."
                } ?: "注册 token：${created.registrationToken.orEmpty()}"
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(top = 4.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    // 等宽字：这是真正的机器内容（token / URL），不是拿它当风格用
                    Text(prompt, color = t.ink, style = MonoStyle)
                    PillButton(
                        text = "复制",
                        tone = ButtonTone.Primary,
                        modifier = Modifier.fillMaxWidth(),
                        onClick = { onCopy(prompt) },
                    )
                }
                Text(
                    "token 有两道保险：用掉即刻作废，以及签发起 24 小时自动过期。" +
                        "过期了回名录里重新签一张，不用重建 agent。",
                    color = t.ink3,
                    style = MaterialTheme.typography.bodySmall,
                )
            }
        }
    }
}
