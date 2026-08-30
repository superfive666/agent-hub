package org.agenthub.app

import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import org.agenthub.app.data.AdminMe
import org.agenthub.app.data.AgentSummary
import org.agenthub.app.data.ApkMeta
import org.agenthub.app.data.BoardResponse
import org.agenthub.app.data.CreateAgentRequest
import org.agenthub.app.data.CreatePostRequest
import org.agenthub.app.data.CreateTodoRequest
import org.agenthub.app.data.CreatedAgent
import org.agenthub.app.data.Health
import org.agenthub.app.data.HubApi
import org.agenthub.app.data.HubException
import org.agenthub.app.data.Prefs
import org.agenthub.app.data.Settings
import org.agenthub.app.data.ThreadDetail
import org.agenthub.app.data.TodoStateRequest
import org.agenthub.app.data.TodoStep
import org.agenthub.app.data.TodoSummary
import org.agenthub.app.data.UpdateAgentRequest
import org.agenthub.app.data.SessionCookieJar
import org.agenthub.app.ui.theme.ThemeMode
import org.agenthub.core.HubUrlResult
import org.agenthub.core.OutboxState
import org.agenthub.core.normalizeHubUrl
import org.agenthub.core.outboxStateOf
import org.agenthub.core.zoneOf
import java.time.ZoneId

/** 一次异步取数的三态。**「还没开始」和「加载中」要分开** —— 界面上一个是空白，一个是骨架。 */
sealed interface Load<out T> {
    data object Idle : Load<Nothing>
    data object Loading : Load<Nothing>
    data class Ok<T>(val value: T) : Load<T>
    data class Err(val message: String, val unauthorized: Boolean = false) : Load<Nothing>
}

/**
 * 取值，没有就 null。
 *
 * **用 `when` + 智能转换，不用 `as? Load.Ok`** —— 后者的类型实参会被擦成
 * `Load.Ok<*>`，`.value` 于是是 `Any?`，赋给 `List<TodoSummary>` 直接编不过。
 * 这个坑很容易在补一个新页面时重新踩一遍，所以取值只走这一个入口。
 */
fun <T> Load<T>.valueOrNull(): T? = when (this) {
    is Load.Ok -> value
    else -> null
}

/** app 现在应该显示哪一屏。**顺序就是判断顺序**，不能颠倒。 */
enum class Gate {
    /** 还在读本地那两个字符串。第一帧用它，不闪。 */
    Booting,

    /** 没填过 hub 地址 —— app 独有的第一屏，网页上不存在这一步。 */
    NeedHub,

    /** 有地址没会话。 */
    NeedLogin,

    /** 进得去。 */
    Ready,
}

/**
 * 全局状态。
 *
 * 一个 ViewModel 管全部页面，是因为这些页面共享的东西太多了
 * （会话、平台时区、outbox 健康度、名录）—— 拆成七个 ViewModel 之后，
 * 光是把这四样传来传去就比现在的代码多。
 *
 * **outbox 健康度在这里而不是设置页里**，因为 §1.4 要求它出现在**每一个**页面上。
 */
class AppViewModel(
    private val api: HubApi,
    private val prefs: Prefs,
    private val cookieJar: SessionCookieJar,
) : ViewModel() {

    private val _gate = MutableStateFlow(Gate.Booting)
    val gate: StateFlow<Gate> = _gate.asStateFlow()

    private val _hubUrl = MutableStateFlow("")
    val hubUrl: StateFlow<String> = _hubUrl.asStateFlow()

    private val _insecureHub = MutableStateFlow(false)
    val insecureHub: StateFlow<Boolean> = _insecureHub.asStateFlow()

    private val _me = MutableStateFlow<AdminMe?>(null)
    val me: StateFlow<AdminMe?> = _me.asStateFlow()

    private val _themeMode = MutableStateFlow(ThemeMode.System)
    val themeMode: StateFlow<ThemeMode> = _themeMode.asStateFlow()

    private val _loginError = MutableStateFlow<String?>(null)
    val loginError: StateFlow<String?> = _loginError.asStateFlow()

    private val _loggingIn = MutableStateFlow(false)
    val loggingIn: StateFlow<Boolean> = _loggingIn.asStateFlow()

    private val _todos = MutableStateFlow<Load<List<TodoSummary>>>(Load.Idle)
    val todos: StateFlow<Load<List<TodoSummary>>> = _todos.asStateFlow()

    private val _directory = MutableStateFlow<Load<List<AgentSummary>>>(Load.Idle)
    val directory: StateFlow<Load<List<AgentSummary>>> = _directory.asStateFlow()

    private val _thread = MutableStateFlow<Load<ThreadDetail>>(Load.Idle)
    val thread: StateFlow<Load<ThreadDetail>> = _thread.asStateFlow()

    private val _steps = MutableStateFlow<Load<List<TodoStep>>>(Load.Idle)
    val steps: StateFlow<Load<List<TodoStep>>> = _steps.asStateFlow()

    private val _board = MutableStateFlow<Load<BoardResponse>>(Load.Idle)
    val board: StateFlow<Load<BoardResponse>> = _board.asStateFlow()

    private val _settings = MutableStateFlow<Load<Settings>>(Load.Idle)
    val settings: StateFlow<Load<Settings>> = _settings.asStateFlow()

    private val _apk = MutableStateFlow<ApkMeta?>(null)
    val apk: StateFlow<ApkMeta?> = _apk.asStateFlow()

    /**
     * outbox 状态。**初始值是 Unknown 而不是 Quiet。**
     *
     * Quiet 的意思是"我查过了，没问题"；启动时我们还没查过。
     * 用 Quiet 起手的话，从启动到第一次 health 返回之间那几秒，
     * 界面在陈述一件它并不知道的事 —— 而这正是 §1.4 要防的失败模式。
     */
    private val _outbox = MutableStateFlow<OutboxState>(OutboxState.Unknown)
    val outbox: StateFlow<OutboxState> = _outbox.asStateFlow()

    private val _newAgent = MutableStateFlow<CreatedAgent?>(null)
    val newAgent: StateFlow<CreatedAgent?> = _newAgent.asStateFlow()

    /**
     * 名录上一次管理动作的结果，一句话。
     *
     * 存在的理由是 409 `agent_in_use`：**它不是异常，是删除的正常结果之一**。
     * 界面要把它翻译成「这个 agent 背着 3 条 todo，改用停用」，
     * 而不是弹一句「删除失败」让人自己去猜卡在哪。
     */
    private val _agentNotice = MutableStateFlow<String?>(null)
    val agentNotice: StateFlow<String?> = _agentNotice.asStateFlow()

    /** 平台时区。看板按它切天，**不用设备时区**（见 core/Dates.kt）。 */
    val zone: ZoneId get() = zoneOf(_me.value?.timezone ?: _settings.value.valueOrNull()?.timezone)

    init {
        viewModelScope.launch {
            prefs.themeMode.collect { _themeMode.value = it }
        }
        viewModelScope.launch {
            val url = prefs.hubUrlNow()
            if (url.isNullOrBlank()) {
                _gate.value = Gate.NeedHub
                return@launch
            }
            _hubUrl.value = url
            _insecureHub.value = (normalizeHubUrl(url) as? HubUrlResult.Ok)?.insecure == true
            api.baseUrl = url
            refreshSession()
        }
    }

    // ── hub 地址 ──

    /** @return 出错时那句能直接显示给用户的话；成功时 null。 */
    suspend fun setHubUrl(input: String): String? =
        when (val r = normalizeHubUrl(input)) {
            is HubUrlResult.Invalid -> r.error
            is HubUrlResult.Ok -> {
                // 换 hub 必须连会话一起清 —— cookie 是签给某一台 hub 的，
                // 揣着旧的去打新的会全程 401，而用户会以为是密码错了。
                prefs.switchHub(r.baseUrl)
                cookieJar.set(null)
                api.baseUrl = r.baseUrl
                _hubUrl.value = r.baseUrl
                _insecureHub.value = r.insecure
                _me.value = null
                _gate.value = Gate.NeedLogin
                null
            }
        }

    /**
     * 忘掉当前 hub，回到第一屏。
     *
     * **连会话一起清**：cookie 是签给某一台 hub 的，留着它下次填了别的地址
     * 会全程 401，而用户会以为是密码错了。
     */
    suspend fun forgetHub() {
        prefs.switchHub("")
        cookieJar.set(null)
        api.baseUrl = ""
        _hubUrl.value = ""
        _insecureHub.value = false
        _me.value = null
        _loginError.value = null
        _gate.value = Gate.NeedHub
    }

    // ── 会话 ──

    /**
     * OIDC 的授权入口地址。**给 WebView 用，不是给 OkHttp 用** ——
     * 这条路必须让一个能收 cookie 的浏览器上下文自己去跟 302。
     */
    fun oidcStartUrl(): String = api.oidcStartUrl()

    private suspend fun refreshSession() {
        _gate.value = try {
            _me.value = api.me()
            Gate.Ready
        } catch (e: HubException) {
            _me.value = null
            Gate.NeedLogin
        }
    }

    fun login(username: String, password: String) {
        viewModelScope.launch {
            _loggingIn.value = true
            _loginError.value = null
            try {
                api.login(username, password)
                refreshSession()
                if (_gate.value == Gate.Ready) refreshAll()
            } catch (e: HubException) {
                // 401 刻意不区分「用户名不存在」和「密码错误」—— 区分就是在帮人枚举。
                // 后端已经这么做了，这里只是把它那句话原样交出去。
                _loginError.value = if (e.unauthorized) {
                    e.error?.message ?: "凭据不对。此实例只有预置的那个账号能进来。"
                } else {
                    e.message
                }
            } finally {
                _loggingIn.value = false
            }
        }
    }

    /** OIDC 走完之后，WebView 那边把 cookie 交过来。 */
    fun onOidcSession(cookie: String) {
        cookieJar.set(cookie)
        viewModelScope.launch {
            refreshSession()
            if (_gate.value == Gate.Ready) refreshAll()
        }
    }

    fun logout() {
        viewModelScope.launch {
            runCatching { api.logout() }
            cookieJar.set(null)
            _me.value = null
            _gate.value = Gate.NeedLogin
            _todos.value = Load.Idle
            _directory.value = Load.Idle
            _thread.value = Load.Idle
            _outbox.value = OutboxState.Unknown
        }
    }

    fun setThemeMode(mode: ThemeMode) {
        viewModelScope.launch { prefs.setThemeMode(mode) }
    }

    // ── 取数 ──

    fun refreshAll() {
        refreshTodos()
        refreshHealth()
        refreshDirectory()
    }

    private fun onUnauthorized() {
        // 会话过期（12 小时）时每个请求都会 401。**把人送回登录页**，
        // 而不是在每个页面上各弹一句"请求失败" —— 那样用户要点七次才明白。
        _me.value = null
        _gate.value = Gate.NeedLogin
    }

    private inline fun <T> load(
        state: MutableStateFlow<Load<T>>,
        crossinline block: suspend () -> T,
    ) {
        state.value = Load.Loading
        viewModelScope.launch {
            state.value = try {
                Load.Ok(block())
            } catch (e: HubException) {
                if (e.unauthorized) onUnauthorized()
                Load.Err(e.message, e.unauthorized)
            }
        }
    }

    fun refreshTodos() = load(_todos) { api.todos() }
    fun refreshDirectory() = load(_directory) { api.directory() }
    fun refreshSettings() = load(_settings) { api.settings() }
    fun refreshBoard(date: String, groupBy: String) = load(_board) { api.board(date, groupBy) }

    fun openThread(threadId: String) {
        load(_thread) { api.thread(threadId) }
        load(_steps) { api.steps(threadId) }
    }

    fun refreshHealth() {
        viewModelScope.launch {
            _outbox.value = try {
                val h: Health = api.health()
                outboxStateOf(
                    healthy = true,
                    workerAlive = h.workerAlive,
                    lagSeconds = h.outboxLagSeconds,
                    pending = h.outboxPending,
                )
            } catch (e: HubException) {
                if (e.unauthorized) onUnauthorized()
                // 拿不到就是 Unknown，**不是 Quiet** —— 见 _outbox 的注释。
                OutboxState.Unknown
            }
        }
    }

    fun refreshApkMeta() {
        viewModelScope.launch {
            // 公开端点，失败了就当没包。这一条不该让设置页整页报错。
            _apk.value = runCatching { api.apkMeta() }.getOrNull()
        }
    }

    // ── 写 ──

    fun createTodo(req: CreateTodoRequest, onDone: (String?) -> Unit) {
        viewModelScope.launch {
            try {
                val t = api.createTodo(req)
                refreshTodos()
                onDone(t.threadId)
            } catch (e: HubException) {
                if (e.unauthorized) onUnauthorized()
                _todos.value = Load.Err(e.message, e.unauthorized)
                onDone(null)
            }
        }
    }

    fun reply(threadId: String, body: String, mentions: List<String>) {
        viewModelScope.launch {
            runCatching { api.createPost(threadId, CreatePostRequest(body, mentions)) }
            openThread(threadId)
        }
    }

    /**
     * 确认放行。**这个动作只有人做得了**（ADR-0008）——
     * agent 撞上闸门时该做的是在 thread 里把需求问清楚，然后等 todo.approved。
     */
    fun confirmTodo(threadId: String) {
        viewModelScope.launch {
            runCatching { api.setTodoState(threadId, TodoStateRequest(confirmed = true)) }
            openThread(threadId)
            refreshTodos()
        }
    }

    fun setTodoStatus(threadId: String, status: String) {
        viewModelScope.launch {
            runCatching { api.setTodoState(threadId, TodoStateRequest(status = status)) }
            openThread(threadId)
            refreshTodos()
        }
    }

    fun createAgent(name: String, summary: String?) {
        viewModelScope.launch {
            _newAgent.value = runCatching {
                api.createAgent(CreateAgentRequest(name = name, summary = summary, issueToken = true))
            }.getOrNull()
            refreshDirectory()
        }
    }

    fun clearNewAgent() {
        _newAgent.value = null
    }

    fun clearAgentNotice() {
        _agentNotice.value = null
    }

    /** 补签一张注册 token。原来那张用掉即作废、24 小时也会自己过期。 */
    fun reissueToken(agentId: String) {
        viewModelScope.launch {
            try {
                _newAgent.value = api.issueRegistrationToken(agentId)
            } catch (e: HubException) {
                _agentNotice.value = e.message
            }
        }
    }

    /**
     * 停用 / 启用。
     *
     * **停用是立刻生效的下线，不是一个标签** —— 凭证校验要求 status='active'，
     * 状态一改这个 agent 的凭证当场就认证不过。和「吊销凭证」的区别是**可逆**：
     * 凭证还留着，重新启用就能继续用，不必重走注册换证。
     */
    fun setAgentEnabled(agentId: String, enabled: Boolean) {
        viewModelScope.launch {
            try {
                api.updateAgent(agentId, UpdateAgentRequest(enabled = enabled))
                _agentNotice.value = if (enabled) "已启用" else "已停用 —— 它的凭证现在起认证不过，但还留着"
                refreshDirectory()
            } catch (e: HubException) {
                _agentNotice.value = e.message
            }
        }
    }

    /**
     * 删除。**只有没留过痕的 agent 删得掉。**
     *
     * 有 todo/tweet/step 的会 409 —— 那不是失败，那是这条硬约束在说话：
     * 一条 todo 必须有且只有一个主 agent，抹掉作者等于篡改历史。
     * 这时候的正确动作是停用。
     */
    fun deleteAgent(agentId: String) {
        viewModelScope.launch {
            try {
                api.deleteAgent(agentId)
                _agentNotice.value = "已删除"
                refreshDirectory()
            } catch (e: HubException) {
                _agentNotice.value = if (e.error?.code == "agent_in_use") {
                    "这个 agent 有历史留痕，删不掉 —— 改用停用：它会立刻下线，历史一条不动。"
                } else {
                    e.message
                }
            }
        }
    }

    class Factory(
        private val api: HubApi,
        private val prefs: Prefs,
        private val cookieJar: SessionCookieJar,
    ) : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T =
            AppViewModel(api, prefs, cookieJar) as T
    }
}
