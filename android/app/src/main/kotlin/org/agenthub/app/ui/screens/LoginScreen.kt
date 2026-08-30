package org.agenthub.app.ui.screens

import android.annotation.SuppressLint
import android.webkit.CookieManager
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
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
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import org.agenthub.app.data.SESSION_COOKIE
import org.agenthub.app.ui.components.ButtonTone
import org.agenthub.app.ui.components.Chip
import org.agenthub.app.ui.components.ChipTone
import org.agenthub.app.ui.components.PillButton
import org.agenthub.app.ui.components.PillField
import org.agenthub.app.ui.components.Seg
import org.agenthub.app.ui.components.Separator
import org.agenthub.app.ui.glass.Inset
import org.agenthub.app.ui.glass.Pane
import org.agenthub.app.ui.glass.Stage
import org.agenthub.app.ui.glass.StagePadding
import org.agenthub.app.ui.theme.tokens

/**
 * 登录 —— **和控制台完全一样的两种入口，完全一样的会话**。
 *
 * 未登录时 `/api/admin/me` 是 401，拿不到 `authMode`，所以客户端**没法预先
 * 知道这台实例开的是哪种模式**。和 web 一样：两种入口都摆出来，
 * 由实例自己拒绝不属于它的那一种。
 */
@Composable
fun LoginScreen(
    hubUrl: String,
    insecure: Boolean,
    oidcUrl: String,
    error: String?,
    busy: Boolean,
    onLogin: (String, String) -> Unit,
    onOidcSession: (String) -> Unit,
    onChangeHub: () -> Unit,
) {
    val t = tokens()
    var mode by remember { mutableStateOf("password") }
    var username by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var reveal by remember { mutableStateOf(false) }
    var oidcOpen by remember { mutableStateOf(false) }

    if (oidcOpen) {
        OidcWebView(
            startUrl = oidcUrl,
            hubUrl = hubUrl,
            onSession = { oidcOpen = false; onOidcSession(it) },
            onCancel = { oidcOpen = false },
        )
        return
    }

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
                    verticalArrangement = Arrangement.spacedBy(16.dp),
                ) {
                    Text("agent‑hub", color = t.ink, style = MaterialTheme.typography.displaySmall)
                    Text(
                        "此实例只有一个管理员。凭据在部署时预置，不在名单内的账号无法进入。",
                        color = t.ink3,
                        style = MaterialTheme.typography.bodySmall,
                    )

                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Chip(hubUrl.removePrefix("https://").removePrefix("http://"))
                        PillButton(
                            text = "换一台",
                            tone = ButtonTone.Ghost,
                            onClick = onChangeHub,
                        )
                    }

                    // 明文 http 到公网：**说出来，但不拦**。局域网自建是正常形态，
                    // 拦下来等于让一部分人根本用不了；不说的话用户不知道
                    // 自己的管理员口令正在明文过网。
                    if (insecure) {
                        Chip("这台 hub 走的是明文 http —— 你的口令会以明文发出去", tone = ChipTone.Warn)
                    }

                    Inset(modifier = Modifier.fillMaxWidth()) {
                        Column(
                            modifier = Modifier.padding(18.dp),
                            verticalArrangement = Arrangement.spacedBy(14.dp),
                        ) {
                            Seg(
                                options = listOf("password" to "密码", "oidc" to "Google 账号"),
                                value = mode,
                                onValueChange = { mode = it },
                                modifier = Modifier.fillMaxWidth(),
                            )

                            if (mode == "oidc") {
                                Text(
                                    "会打开 Google 的授权页，回来时会话已经种好。\n" +
                                        "一个实例只开一种模式 —— 如果这台是口令模式，这条路会被拒。",
                                    color = t.ink2,
                                    style = MaterialTheme.typography.bodySmall,
                                )
                                PillButton(
                                    text = "用 Google 登录",
                                    tone = ButtonTone.Primary,
                                    modifier = Modifier.fillMaxWidth(),
                                    onClick = { oidcOpen = true },
                                )
                            } else {
                                Text("用户名", color = t.ink3, style = MaterialTheme.typography.labelSmall)
                                PillField(
                                    value = username,
                                    onValueChange = { username = it },
                                    modifier = Modifier.fillMaxWidth(),
                                )
                                Text("密码", color = t.ink3, style = MaterialTheme.typography.labelSmall)
                                PillField(
                                    value = password,
                                    onValueChange = { password = it },
                                    modifier = Modifier.fillMaxWidth(),
                                    visualTransformation = if (reveal) {
                                        VisualTransformation.None
                                    } else {
                                        PasswordVisualTransformation()
                                    },
                                    keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password),
                                    trailing = {
                                        PillButton(
                                            text = if (reveal) "隐藏" else "显示",
                                            tone = ButtonTone.Ghost,
                                            onClick = { reveal = !reveal },
                                        )
                                    },
                                )
                                if (error != null) {
                                    Chip(error, tone = ChipTone.Alert)
                                }
                                PillButton(
                                    text = if (busy) "正在进入…" else "进入控制台",
                                    tone = ButtonTone.Primary,
                                    enabled = !busy,
                                    modifier = Modifier.fillMaxWidth(),
                                    onClick = { onLogin(username, password) },
                                )
                            }
                        }
                    }

                    Separator()
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        Chip("会话写在 HttpOnly Cookie", tone = ChipTone.Neutral)
                    }
                }
            }
        }
    }
}

/**
 * OIDC 走**应用内 WebView，不是 Custom Tabs**。
 *
 * 这是整个 app 里唯一一处「和浏览器不一样」的地方，理由很具体：
 * **Custom Tabs 的 cookie 落在浏览器进程里，app 拿不到。** 授权流程能走完，
 * 但走完之后 app 手上还是没有会话 —— 用户会看到"登录成功"然后被弹回登录页。
 * 只有 WebView 的 [CookieManager] 能把 `hub_session` 交给 OkHttp。
 *
 * ## 安全边界
 *
 * - **只在这一个 WebView 里做这件事**，用完就销毁。
 * - **不注入任何 JS 桥**（没有 `addJavascriptInterface`）—— 授权页面是 Google
 *   的，给它一个通往 app 的接口没有任何理由。
 * - **只从用户自己填的那台 hub 的域名上取 cookie**，不从 accounts.google.com 取。
 * - JS 必须开着：Google 的授权页离了它跑不起来。这不是可以关掉的选项。
 */
@SuppressLint("SetJavaScriptEnabled")
@Composable
private fun OidcWebView(
    startUrl: String,
    hubUrl: String,
    onSession: (String) -> Unit,
    onCancel: () -> Unit,
) {
    val t = tokens()
    Stage {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .statusBarsPadding()
                .navigationBarsPadding()
                .padding(StagePadding),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    "Google 授权",
                    color = t.ink,
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.weight(1f),
                )
                PillButton(text = "取消", tone = ButtonTone.Ghost, onClick = onCancel)
            }
            Pane(modifier = Modifier.fillMaxSize(), prism = false, sheen = false) {
                AndroidView(
                    modifier = Modifier.fillMaxSize(),
                    factory = { context ->
                        CookieManager.getInstance().setAcceptCookie(true)
                        WebView(context).apply {
                            settings.javaScriptEnabled = true
                            settings.domStorageEnabled = true
                            webViewClient = object : WebViewClient() {
                                override fun onPageFinished(view: WebView?, url: String?) {
                                    // 每次页面停下来都查一次：授权走完之后 hub 会 302
                                    // 回控制台并在这一跳把 cookie 种上。
                                    val raw = CookieManager.getInstance().getCookie(hubUrl)
                                    val session = raw
                                        ?.split(';')
                                        ?.map(String::trim)
                                        ?.firstOrNull { it.startsWith("$SESSION_COOKIE=") }
                                        ?.substringAfter('=')
                                        ?.takeIf { it.isNotBlank() }
                                    if (session != null) onSession(session)
                                }
                            }
                            loadUrl(startUrl)
                        }
                    },
                )
            }
        }
    }
}
