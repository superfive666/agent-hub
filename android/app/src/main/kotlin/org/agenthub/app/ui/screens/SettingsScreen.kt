package org.agenthub.app.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import org.agenthub.app.Load
import org.agenthub.app.valueOrNull
import org.agenthub.app.data.AdminMe
import org.agenthub.app.data.ApkMeta
import org.agenthub.app.data.Settings
import org.agenthub.app.ui.components.ButtonTone
import org.agenthub.app.ui.components.Chip
import org.agenthub.app.ui.components.ChipTone
import org.agenthub.app.ui.components.PageHeader
import org.agenthub.app.ui.components.PillButton
import org.agenthub.app.ui.components.Seg
import org.agenthub.app.ui.components.Separator
import org.agenthub.app.ui.glass.Inset
import org.agenthub.app.ui.theme.MonoStyle
import org.agenthub.app.ui.theme.ThemeMode
import org.agenthub.app.ui.theme.tokens
import org.agenthub.core.latencyLabel
import org.agenthub.core.maskEmail

/**
 * 设置。这一页是**只读的**（除了主题和 hub 地址）——
 * 部署级配置由 `.env` 决定，改配置要重新部署。
 *
 * 在 app 里给它们做成可编辑的输入框会是一个谎：填了、点了保存、
 * 然后什么都没发生。
 */
@Composable
fun ColumnScope.SettingsScreen(
    me: AdminMe?,
    settings: Load<Settings>,
    apk: ApkMeta?,
    hubUrl: String,
    themeMode: ThemeMode,
    version: String,
    onBack: () -> Unit,
    onThemeMode: (ThemeMode) -> Unit,
    onChangeHub: () -> Unit,
    onLogout: () -> Unit,
    onRefresh: () -> Unit,
) {
    val t = tokens()
    LaunchedEffect(Unit) { onRefresh() }

    PageHeader(title = "系统设置", subtitle = "部署级配置与运行状态 · 只读")
    Row(modifier = Modifier.padding(horizontal = 14.dp)) {
        PillButton(text = "← 返回", tone = ButtonTone.Ghost, onClick = onBack)
    }

    Inset(modifier = Modifier.weight(1f).fillMaxWidth().padding(14.dp)) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Section("外观") {
                Seg(
                    options = listOf("system" to "跟随系统", "light" to "亮色", "dark" to "暗色"),
                    value = when (themeMode) {
                        ThemeMode.System -> "system"
                        ThemeMode.Light -> "light"
                        ThemeMode.Dark -> "dark"
                    },
                    onValueChange = {
                        onThemeMode(
                            when (it) {
                                "light" -> ThemeMode.Light
                                "dark" -> ThemeMode.Dark
                                else -> ThemeMode.System
                            },
                        )
                    },
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    "系统开了「移除动画」时，app 里所有动效会自动停掉。",
                    color = t.ink3,
                    style = MaterialTheme.typography.bodySmall,
                )
            }

            Section("这台 hub") {
                Text(hubUrl, color = t.ink, style = MonoStyle)
                PillButton(text = "换一台 hub", tone = ButtonTone.Default, onClick = onChangeHub)
                Text(
                    "换 hub 会同时清掉当前会话 —— 会话是签给某一台 hub 的，" +
                        "带着旧的去打新的会全程被拒。",
                    color = t.ink3,
                    style = MaterialTheme.typography.bodySmall,
                )
            }

            Section("管理员") {
                KeyValue("认证方式", if (me?.authMode == "oidc") "Google OIDC" else "用户名密码")
                // OIDC 模式下这是一串 Google 邮箱，整串画出来会把卡片撑破。
                // 打码只缩短显示，域名完整保留 —— 用户要靠它确认登的是哪个账号。
                KeyValue("账号", maskEmail(me?.username))
                Text(
                    "凭据在部署时注入，不能在这里改。没有预置管理员时服务会启动失败，" +
                        "不会悄悄跑起一个谁都能进的实例。",
                    color = t.ink3,
                    style = MaterialTheme.typography.bodySmall,
                )
                PillButton(text = "退出登录", tone = ButtonTone.Danger, onClick = onLogout)
            }

            val s = settings.valueOrNull()
            if (s != null) {
                Section("平台") {
                    KeyValue("时区", s.timezone)
                    Text(
                        "决定看板按哪个时区切分「一天」。app 用的就是这个，不是手机的时区 —— " +
                            "否则同一条 todo 在不同城市会落到不同的日期上。",
                        color = t.ink3,
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
                Section("通道") {
                    KeyValue("长轮询超时", "${s.longPollMaxSeconds} 秒")
                    KeyValue("inbox 保留", "${s.inboxRetentionDays} 天")
                    KeyValue("在线判定 · 长轮询", latencyLabel(s.onlineWindowSeconds.longpoll))
                    KeyValue("在线判定 · webhook", latencyLabel(s.onlineWindowSeconds.webhook))
                    // cron 档的窗口比别的档大一个数量级。用同一个窗口判的话，
                    // 每个 cron agent 都会被画成离线。
                    KeyValue("在线判定 · cron", latencyLabel(s.onlineWindowSeconds.cron))
                }
            }

            Section("这个 app") {
                KeyValue("版本", version)
                if (apk?.available == true) {
                    KeyValue("hub 上发布的", apk.version ?: "—")
                    if (apk.version != null && apk.version != version) {
                        // 只提示，不自动下载 —— 自建分发下"自动更新"要用户
                        // 一路点安装未知来源，不如让他自己决定什么时候去下。
                        Chip("hub 上有个不一样的版本，可以去控制台下载", tone = ChipTone.Warn)
                    }
                }
                Text(
                    "这个 app 只是一个前端：不存业务数据、不做本地决策，" +
                        "所有正确性都在 hub 那边。",
                    color = t.ink3,
                    style = MaterialTheme.typography.bodySmall,
                )
            }
        }
    }
}

@Composable
private fun Section(title: String, content: @Composable ColumnScope.() -> Unit) {
    val t = tokens()
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(title, color = t.ink3, style = MaterialTheme.typography.labelSmall)
        content()
        Separator()
    }
}

@Composable
private fun KeyValue(key: String, value: String) {
    val t = tokens()
    Row(
        modifier = Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(key, color = t.ink2, style = MaterialTheme.typography.bodySmall, modifier = Modifier.weight(1f))
        Text(value, color = t.ink, style = MaterialTheme.typography.titleSmall)
    }
}
