package org.agenthub.app.nav

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import kotlinx.coroutines.launch
import androidx.compose.runtime.rememberCoroutineScope
import org.agenthub.app.AppViewModel
import org.agenthub.app.BuildConfig
import org.agenthub.app.Gate
import org.agenthub.app.Load
import org.agenthub.app.valueOrNull
import org.agenthub.core.OutboxState
import org.agenthub.app.ui.components.AppScaffold
import org.agenthub.app.ui.components.Tab
import org.agenthub.app.ui.screens.BoardScreen
import org.agenthub.app.ui.screens.DirectoryScreen
import org.agenthub.app.ui.screens.LoginScreen
import org.agenthub.app.ui.screens.NewAgentScreen
import org.agenthub.app.ui.screens.NewTodoScreen
import org.agenthub.app.ui.screens.SettingsScreen
import org.agenthub.app.ui.screens.SetupScreen
import org.agenthub.app.ui.screens.ThreadScreen
import org.agenthub.app.ui.screens.ThreadsScreen
import org.agenthub.app.ui.screens.TodosScreen

private object Routes {
    const val THREADS = "threads"
    const val THREAD = "thread/{threadId}"
    const val BOARD = "board"
    const val TODOS = "todos"
    const val DIRECTORY = "directory"
    const val SETTINGS = "settings"
    const val NEW_TODO = "todos/new"
    const val NEW_AGENT = "directory/new"

    fun thread(id: String) = "thread/$id"
}

/**
 * 三道闸门，**顺序不能颠倒**：
 *
 * 1. 还在读本地状态 → 什么都不画（不闪）
 * 2. 没填过 hub 地址 → 只能填地址
 * 3. 有地址没会话 → 只能登录
 * 4. 都有 → 正常导航
 *
 * 把 2 和 3 调过来的话，新装的用户会先看到一个「往哪登录？」的登录页。
 */
@Composable
fun AppNav(vm: AppViewModel) {
    val gate by vm.gate.collectAsStateWithLifecycle()
    val hubUrl by vm.hubUrl.collectAsStateWithLifecycle()
    val insecure by vm.insecureHub.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    when (gate) {
        Gate.Booting -> Unit

        Gate.NeedHub -> SetupScreen(initial = hubUrl, onSubmit = vm::setHubUrl)

        Gate.NeedLogin -> {
            val error by vm.loginError.collectAsStateWithLifecycle()
            val busy by vm.loggingIn.collectAsStateWithLifecycle()
            LoginScreen(
                hubUrl = hubUrl,
                insecure = insecure,
                oidcUrl = vm.oidcStartUrl(),
                error = error,
                busy = busy,
                onLogin = vm::login,
                onOidcSession = vm::onOidcSession,
                onChangeHub = { scope.launch { vm.forgetHub() } },
            )
        }

        Gate.Ready -> ReadyNav(vm)
    }
}

@Composable
private fun ReadyNav(vm: AppViewModel) {
    val nav = rememberNavController()
    val context = LocalContext.current
    val scope = rememberCoroutineScope()

    val outbox by vm.outbox.collectAsStateWithLifecycle()
    val todos by vm.todos.collectAsStateWithLifecycle()
    val directory by vm.directory.collectAsStateWithLifecycle()
    val thread by vm.thread.collectAsStateWithLifecycle()
    val steps by vm.steps.collectAsStateWithLifecycle()
    val board by vm.board.collectAsStateWithLifecycle()
    val settings by vm.settings.collectAsStateWithLifecycle()
    val apk by vm.apk.collectAsStateWithLifecycle()
    val me by vm.me.collectAsStateWithLifecycle()
    val themeMode by vm.themeMode.collectAsStateWithLifecycle()
    val newAgent by vm.newAgent.collectAsStateWithLifecycle()
    val agentNotice by vm.agentNotice.collectAsStateWithLifecycle()
    val hubUrl by vm.hubUrl.collectAsStateWithLifecycle()

    LaunchedEffect(Unit) { vm.refreshAll() }

    val agents = directory.valueOrNull().orEmpty()
    val mentionPairs = agents.map { it.agentId to it.name }

    NavHost(navController = nav, startDestination = Routes.THREADS) {
        composable(Routes.THREADS) {
            Screen(nav, Tab.Threads, outbox, vm) {
                ThreadsScreen(
                    todos = todos,
                    zone = vm.zone,
                    onOpen = { id -> vm.openThread(id); nav.navigate(Routes.thread(id)) },
                    onNewTodo = { nav.navigate(Routes.NEW_TODO) },
                )
            }
        }

        composable(Routes.BOARD) {
            Screen(nav, Tab.Board, outbox, vm) {
                BoardScreen(
                    board = board,
                    zone = vm.zone,
                    onLoad = vm::refreshBoard,
                    onOpen = { id -> vm.openThread(id); nav.navigate(Routes.thread(id)) },
                )
            }
        }

        composable(Routes.TODOS) {
            Screen(nav, Tab.Todos, outbox, vm) {
                TodosScreen(
                    todos = todos,
                    zone = vm.zone,
                    onOpen = { id -> vm.openThread(id); nav.navigate(Routes.thread(id)) },
                )
            }
        }

        composable(Routes.DIRECTORY) {
            Screen(nav, Tab.Directory, outbox, vm) {
                DirectoryScreen(
                    directory = directory,
                    notice = agentNotice,
                    onNewAgent = { vm.clearNewAgent(); nav.navigate(Routes.NEW_AGENT) },
                    // 补签走的是同一个「把这句话发过去」的页面 —— 明文 token
                    // 只出现这一次，两处用同一个展示能少一个漏掉警告的地方。
                    onReissueToken = { id -> vm.reissueToken(id); nav.navigate(Routes.NEW_AGENT) },
                    onSetEnabled = vm::setAgentEnabled,
                    onDelete = vm::deleteAgent,
                    onDismissNotice = vm::clearAgentNotice,
                )
            }
        }

        composable(Routes.THREAD) { entry ->
            val id = entry.arguments?.getString("threadId").orEmpty()
            // 详情页**不带 tab**：它是从某个 tab 钻进来的一层，
            // 底部继续亮着 tab 会让人以为返回键会退出 app。
            Screen(nav, null, outbox, vm) {
                ThreadScreen(
                    thread = thread,
                    steps = steps,
                    directory = mentionPairs,
                    zone = vm.zone,
                    onBack = { nav.popBackStack() },
                    onReply = { body, mentions -> vm.reply(id, body, mentions) },
                    onConfirm = { vm.confirmTodo(id) },
                    onStatus = { status -> vm.setTodoStatus(id, status) },
                )
            }
        }

        composable(Routes.NEW_TODO) {
            Screen(nav, null, outbox, vm) {
                NewTodoScreen(
                    directory = agents,
                    onBack = { nav.popBackStack() },
                    onCreate = { req ->
                        vm.createTodo(req) { threadId ->
                            nav.popBackStack()
                            if (threadId != null) {
                                vm.openThread(threadId)
                                nav.navigate(Routes.thread(threadId))
                            }
                        }
                    },
                )
            }
        }

        composable(Routes.NEW_AGENT) {
            Screen(nav, null, outbox, vm) {
                NewAgentScreen(
                    created = newAgent,
                    onBack = { nav.popBackStack() },
                    onCreate = vm::createAgent,
                    onDone = { vm.clearNewAgent(); nav.popBackStack() },
                    onCopy = { text -> copyToClipboard(context, text) },
                )
            }
        }

        composable(Routes.SETTINGS) {
            Screen(nav, null, outbox, vm) {
                SettingsScreen(
                    me = me,
                    settings = settings,
                    apk = apk,
                    hubUrl = hubUrl,
                    themeMode = themeMode,
                    version = BuildConfig.VERSION_NAME,
                    onBack = { nav.popBackStack() },
                    onThemeMode = vm::setThemeMode,
                    onChangeHub = { scope.launch { vm.forgetHub() } },
                    onLogout = vm::logout,
                    onRefresh = { vm.refreshSettings(); vm.refreshApkMeta(); vm.refreshHealth() },
                )
            }
        }
    }
}

@Composable
private fun Screen(
    nav: NavHostController,
    tab: Tab?,
    outbox: OutboxState,
    vm: AppViewModel,
    content: @Composable androidx.compose.foundation.layout.ColumnScope.() -> Unit,
) {
    AppScaffold(
        current = tab,
        onTab = { t ->
            // launchSingleTop + popUpTo：四个 tab 之间来回点不该在返回栈里
            // 堆出几十层，否则按返回键要按很久才退得出去。
            nav.navigate(t.route) {
                popUpTo(Routes.THREADS) { inclusive = false }
                launchSingleTop = true
            }
            when (t) {
                Tab.Threads, Tab.Todos -> vm.refreshTodos()
                Tab.Directory -> vm.refreshDirectory()
                Tab.Board -> Unit // 看板自己按日期加载
            }
            vm.refreshHealth()
        },
        outbox = outbox,
        onSettings = if (tab != null) {
            { nav.navigate(Routes.SETTINGS) }
        } else {
            null
        },
        content = content,
    )
}

private fun copyToClipboard(context: Context, text: String) {
    val cm = context.getSystemService(Context.CLIPBOARD_SERVICE) as ClipboardManager
    cm.setPrimaryClip(ClipData.newPlainText("agent-hub", text))
}
