package org.agenthub.app.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CalendarViewWeek
import androidx.compose.material.icons.filled.Checklist
import androidx.compose.material.icons.filled.Forum
import androidx.compose.material.icons.filled.Group
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.draw.clip
import androidx.compose.ui.unit.dp
import org.agenthub.app.ui.glass.Pane
import org.agenthub.app.ui.glass.PaneGap
import org.agenthub.app.ui.glass.Stage
import org.agenthub.app.ui.glass.StagePadding
import org.agenthub.app.ui.theme.tokens
import org.agenthub.core.OutboxState

/** 底部四个 tab。和 web 侧栏一一对应，顺序也一样 —— 换个顺序等于换个产品。 */
enum class Tab(val route: String, val label: String, val icon: ImageVector) {
    Threads("threads", "对话", Icons.Filled.Forum),
    Board("board", "看板", Icons.Filled.CalendarViewWeek),
    Todos("todos", "待办", Icons.Filled.Checklist),
    Directory("directory", "名录", Icons.Filled.Group),
}

/**
 * 主界面骨架 —— 设计语言 §2 的构图搬到手机上。
 *
 * ```
 * 舞台 Stage            有天气的背景
 *   玻璃板 Pane          悬浮其上
 *     嵌套内板 Inset      板中有板（由各页面自己放）
 *   底部 tab 条           **舞台的正常子项，不是浮在上面的 fixed 条**
 * ```
 *
 * ## tab 条为什么是 Column 的正常子项
 *
 * web 上踩过这个坑：写成 `fixed` 之后主板不知道 tab 条占多高，只能拿 padding
 * 去猜，猜出来的值和真实高度（还要加安全区）对不上，两个圆角矩形就会互相
 * 穿插 —— 主板的棱镜边从 tab 条中间横穿过去。
 *
 * Android 上等价的错法是给主板加 `padding(bottom = 72.dp)` 再把 tab 条
 * 绝对定位。这里不那么做：两块板是同一条 Column 的兄弟，中间隔着真正的 gap，
 * 左右共用舞台的同一份内边距，安全区由舞台的 `navigationBarsPadding` 让出来。
 *
 * ## outbox 告警带的位置
 *
 * 在玻璃板**最顶上、标题之前**，每一页都有。§1.4：不可折叠、不可降级、
 * 不可挪到二级页面。会想动它的理由总是"手机屏这么小" —— 那一行正是它的价值。
 */
@Composable
fun AppScaffold(
    current: Tab?,
    onTab: (Tab) -> Unit,
    outbox: OutboxState,
    modifier: Modifier = Modifier,
    onSettings: (() -> Unit)? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    Stage(modifier = modifier) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .statusBarsPadding()
                .navigationBarsPadding()
                .padding(StagePadding),
            verticalArrangement = Arrangement.spacedBy(PaneGap),
        ) {
            Pane(modifier = Modifier.weight(1f).fillMaxWidth()) {
                Column(modifier = Modifier.fillMaxSize()) {
                    OutboxBanner(
                        state = outbox,
                        modifier = Modifier.padding(start = 14.dp, end = 14.dp, top = 14.dp),
                    )
                    content()
                }
            }

            if (current != null) {
                BottomTabs(current = current, onTab = onTab, onSettings = onSettings)
            }
        }
    }
}

@Composable
private fun BottomTabs(current: Tab, onTab: (Tab) -> Unit, onSettings: (() -> Unit)?) {
    val t = tokens()
    // 卡片形状的 tab 条，和主板同一套玻璃语言。sheen 关掉：一条 64dp 的
    // 窄带子上扫光只会看起来像渲染错误。
    Pane(radius = 26.dp, sheen = false, modifier = Modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 6.dp, vertical = 6.dp),
            horizontalArrangement = Arrangement.SpaceEvenly,
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Tab.entries.forEach { tab ->
                val active = tab == current
                Column(
                    horizontalAlignment = Alignment.CenterHorizontally,
                    modifier = Modifier
                        .clip(RoundedCornerShape(18.dp))
                        .clickable { onTab(tab) }
                        // 触控目标 ≥ 44dp（§4）。手指不是鼠标指针。
                        .height(MinTouch + 12.dp)
                        .padding(horizontal = 12.dp, vertical = 6.dp),
                    verticalArrangement = Arrangement.Center,
                ) {
                    Icon(
                        imageVector = tab.icon,
                        contentDescription = tab.label,
                        tint = if (active) t.agentInk else t.ink3,
                        modifier = Modifier.size(20.dp),
                    )
                    Text(
                        text = tab.label,
                        color = if (active) t.ink else t.ink3,
                        style = MaterialTheme.typography.labelSmall,
                        modifier = Modifier.padding(top = 3.dp),
                    )
                }
            }
            if (onSettings != null) {
                Column(
                    horizontalAlignment = Alignment.CenterHorizontally,
                    modifier = Modifier
                        .clip(RoundedCornerShape(18.dp))
                        .clickable { onSettings() }
                        .height(MinTouch + 12.dp)
                        .padding(horizontal = 12.dp, vertical = 6.dp),
                    verticalArrangement = Arrangement.Center,
                ) {
                    Icon(
                        imageVector = Icons.Filled.Settings,
                        contentDescription = "系统设置",
                        tint = t.ink3,
                        modifier = Modifier.size(20.dp),
                    )
                    Text(
                        text = "设置",
                        color = t.ink3,
                        style = MaterialTheme.typography.labelSmall,
                        modifier = Modifier.padding(top = 3.dp),
                    )
                }
            }
        }
    }
}

/** 页面标题。所有页面共用，和 web 的 PageHeader 对齐。 */
@Composable
fun PageHeader(
    title: String,
    subtitle: String? = null,
    modifier: Modifier = Modifier,
    actions: (@Composable RowScopeShim.() -> Unit)? = null,
) {
    val t = tokens()
    Row(
        modifier = modifier
            .fillMaxWidth()
            .padding(start = 18.dp, end = 14.dp, top = 16.dp, bottom = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(modifier = Modifier.weight(1f)) {
            Text(title, color = t.ink, style = MaterialTheme.typography.headlineSmall)
            if (subtitle != null) {
                Text(
                    subtitle,
                    color = t.ink3,
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.padding(top = 4.dp),
                )
            }
        }
        actions?.invoke(RowScopeShim)
    }
}

/** 只是为了让 actions 的签名不用暴露 Compose 的 RowScope。 */
object RowScopeShim

/** 一条细分隔线。web 上的 `.sep`。 */
@Composable
fun Separator(modifier: Modifier = Modifier) {
    val t = tokens()
    Box(modifier = modifier.fillMaxWidth().height(1.dp).background(t.hair2))
}
