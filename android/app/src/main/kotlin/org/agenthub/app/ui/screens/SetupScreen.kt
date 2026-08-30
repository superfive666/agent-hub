package org.agenthub.app.ui.screens

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import org.agenthub.app.ui.components.ButtonTone
import org.agenthub.app.ui.components.Chip
import org.agenthub.app.ui.components.ChipTone
import org.agenthub.app.ui.components.PillButton
import org.agenthub.app.ui.components.PillField
import org.agenthub.app.ui.glass.Inset
import org.agenthub.app.ui.glass.Pane
import org.agenthub.app.ui.glass.Stage
import org.agenthub.app.ui.glass.StagePadding
import org.agenthub.app.ui.theme.tokens

/**
 * 第一屏：你的 hub 在哪。
 *
 * **这一屏是 app 独有的，网页上不存在** —— 网页天然知道自己从哪来，
 * app 不知道。它也是这个 app 唯一一处"要用户填技术细节"的地方，
 * 所以要把话说到位：填什么、从哪找、填错了会怎样。
 *
 * 规整逻辑全在 core 的 `normalizeHubUrl` 里（并且有单测）：
 * 用户可以只打 `hub.example.com`，也可以从浏览器地址栏整条粘过来。
 */
@Composable
fun SetupScreen(
    initial: String,
    onSubmit: suspend (String) -> String?,
) {
    val t = tokens()
    var input by remember { mutableStateOf(initial) }
    var error by remember { mutableStateOf<String?>(null) }
    val scope = rememberCoroutineScope()

    Stage {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .statusBarsPadding()
                .navigationBarsPadding()
                .imePadding()
                .padding(StagePadding),
        ) {
            Pane(modifier = Modifier.fillMaxSize()) {
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .verticalScroll(rememberScrollState())
                        .padding(22.dp),
                    verticalArrangement = Arrangement.spacedBy(18.dp, Alignment.CenterVertically),
                    horizontalAlignment = Alignment.Start,
                ) {
                    Text("连到你的 hub", color = t.ink, style = MaterialTheme.typography.displaySmall)
                    Text(
                        "这个 app 只是一个前端 —— 所有数据都在你自己的 hub 上，" +
                            "它不存任何业务数据，也不连别的服务。",
                        color = t.ink3,
                        style = MaterialTheme.typography.bodySmall,
                    )

                    Inset(modifier = Modifier.fillMaxWidth()) {
                        Column(
                            modifier = Modifier.padding(18.dp),
                            verticalArrangement = Arrangement.spacedBy(12.dp),
                        ) {
                            Text("hub 地址", color = t.ink3, style = MaterialTheme.typography.labelSmall)
                            PillField(
                                value = input,
                                onValueChange = { input = it; error = null },
                                placeholder = "hub.example.com",
                                modifier = Modifier.fillMaxWidth(),
                                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                            )
                            Text(
                                "不写协议默认按 https。局域网自建的写全，比如 " +
                                    "http://192.168.1.5:8080 —— 那种情况下会提示你口令是明文发出去的。",
                                color = t.ink3,
                                style = MaterialTheme.typography.bodySmall,
                            )
                            if (error != null) {
                                Chip(error!!, tone = ChipTone.Alert)
                            }
                            PillButton(
                                text = "继续",
                                tone = ButtonTone.Primary,
                                modifier = Modifier.fillMaxWidth(),
                                onClick = {
                                    scope.launch { error = onSubmit(input) }
                                },
                            )
                        }
                    }

                    Text(
                        "地址就是你平时打开控制台的那一个。从浏览器地址栏整条复制过来也行，" +
                            "多余的路径会自动去掉。",
                        color = t.ink3,
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
            }
        }
    }
}
