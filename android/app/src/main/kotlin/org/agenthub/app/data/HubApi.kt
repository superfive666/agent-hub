package org.agenthub.app.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.KSerializer
import kotlinx.serialization.builtins.serializer
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.agenthub.core.joinUrl
import java.io.IOException

/** HTTP 层抛出来的错误。**要能被界面翻译成一句人话**，不是甩一个状态码。 */
class HubException(
    val status: Int,
    val error: ApiError?,
    override val message: String,
    override val cause: Throwable? = null,
) : Exception(message, cause) {

    /** 401 = 没有会话。上层要据此把人送回登录页，而不是弹一句"请求失败"。 */
    val unauthorized: Boolean get() = status == 401

    companion object {
        /** 连不上：地址错了、hub 没起、手机没网。三种在界面上要说得一样清楚。 */
        fun offline(cause: Throwable) = HubException(
            status = 0,
            error = null,
            message = "连不上 hub —— 检查地址和网络，或者这台 hub 现在没在跑",
            cause = cause,
        )
    }
}

private val JSON_MEDIA = "application/json; charset=utf-8".toMediaType()

/**
 * hub 的 admin 接口。
 *
 * **只打 `/api/admin/` 下的那套。** agent 侧那套 Bearer 凭证不出现在这个 app 里 ——
 * ⚠️ 别在 KDoc 里写 `/api/admin/` 加通配星号：Kotlin 的块注释**是嵌套的**，
 * 那个 `/` 加 `*` 会当场开一个新的注释层，把整个文件从这里起全吞掉 ——
 * 报错是文件末尾一句 `Unclosed comment`，而真正的原因在几百行以外。
 * app 是给那一个人类管理员用的，不是给 agent 用的（立项书 §2）。
 */
class HubApi(
    private val client: OkHttpClient,
    /** 已经过 core 的 `normalizeHubUrl` 规整。这里不再猜格式。 */
    @Volatile var baseUrl: String,
) {

    // ── 会话 ──

    suspend fun login(username: String, password: String) {
        post("/api/admin/login", LoginRequest(username, password), LoginRequest.serializer())
    }

    suspend fun logout() {
        postEmpty("/api/admin/logout")
    }

    suspend fun me(): AdminMe = get("/api/admin/me", AdminMe.serializer())

    /**
     * OIDC 的授权入口。**返回地址而不是发请求** —— 这条路必须让一个能收
     * cookie 的浏览器上下文自己去跟 302，用 OkHttp 拿不到最终的会话。
     * 见 `ui/screens/LoginScreen.kt` 里为什么用 WebView 而不是 Custom Tabs。
     */
    fun oidcStartUrl(): String = joinUrl(baseUrl, "/api/admin/auth/google/start")

    // ── 读 ──

    suspend fun todos(): List<TodoSummary> =
        get("/api/admin/todos", TodosResponse.serializer()).todos

    suspend fun directory(): List<AgentSummary> =
        get("/api/admin/directory", DirectoryResponse.serializer()).agents

    suspend fun thread(threadId: String): ThreadDetail =
        get("/api/admin/threads/$threadId", ThreadDetail.serializer())

    suspend fun steps(threadId: String): List<TodoStep> =
        get("/api/admin/todos/$threadId/steps", StepsResponse.serializer()).steps

    suspend fun board(date: String, groupBy: String): BoardResponse =
        get("/api/admin/board?date=$date&groupBy=$groupBy", BoardResponse.serializer())

    suspend fun settings(): Settings = get("/api/admin/settings", Settings.serializer())

    suspend fun health(): Health = get("/api/admin/health", Health.serializer())

    /** 公开端点，不需要会话。设置页拿它显示"当前发布的版本"。 */
    suspend fun apkMeta(): ApkMeta = get("/download/meta", ApkMeta.serializer())

    // ── 写 ──

    suspend fun createTodo(req: CreateTodoRequest): TodoSummary =
        post("/api/admin/todos", req, CreateTodoRequest.serializer(), TodoSummary.serializer())

    suspend fun createPost(threadId: String, req: CreatePostRequest) {
        post("/api/admin/threads/$threadId/posts", req, CreatePostRequest.serializer())
    }

    suspend fun setTodoState(threadId: String, req: TodoStateRequest) {
        post("/api/admin/todos/$threadId/state", req, TodoStateRequest.serializer())
    }

    suspend fun createAgent(req: CreateAgentRequest): CreatedAgent =
        post("/api/admin/agents", req, CreateAgentRequest.serializer(), CreatedAgent.serializer())

    suspend fun issueRegistrationToken(agentId: String): CreatedAgent =
        postEmptyFor("/api/admin/agents/$agentId/registration-token", CreatedAgent.serializer())

    suspend fun updateAgent(agentId: String, req: UpdateAgentRequest) {
        send("PATCH", "/api/admin/agents/$agentId", req, UpdateAgentRequest.serializer())
    }

    /**
     * 物理删除。**只有没留过痕的 agent 删得掉** —— 有 todo/tweet/step 的会
     * 409 `agent_in_use`，那时正确的动作是停用（[updateAgent] 的 enabled=false）。
     */
    suspend fun deleteAgent(agentId: String) {
        val req = Request.Builder().url(joinUrl(baseUrl, "/api/admin/agents/$agentId")).delete().build()
        execute(req, String.serializer(), allowEmpty = true)
    }

    // ── 底座 ──

    private suspend fun <T> get(path: String, serializer: KSerializer<T>): T =
        execute(Request.Builder().url(joinUrl(baseUrl, path)).get().build(), serializer)

    private suspend fun <B> post(path: String, body: B, bodySer: KSerializer<B>) {
        execute(request(path, body, bodySer), String.serializer(), allowEmpty = true)
    }

    private suspend fun <B, T> post(
        path: String,
        body: B,
        bodySer: KSerializer<B>,
        serializer: KSerializer<T>,
    ): T = execute(request(path, body, bodySer), serializer)

    private suspend fun postEmpty(path: String) {
        val req = Request.Builder()
            .url(joinUrl(baseUrl, path))
            .post("".toRequestBody(JSON_MEDIA))
            .build()
        execute(req, String.serializer(), allowEmpty = true)
    }

    private suspend fun <T> postEmptyFor(path: String, serializer: KSerializer<T>): T {
        val req = Request.Builder()
            .url(joinUrl(baseUrl, path))
            .post("".toRequestBody(JSON_MEDIA))
            .build()
        return execute(req, serializer)
    }

    private suspend fun <B> send(method: String, path: String, body: B, bodySer: KSerializer<B>) {
        val req = Request.Builder()
            .url(joinUrl(baseUrl, path))
            .method(method, HubJson.encodeToString(bodySer, body).toRequestBody(JSON_MEDIA))
            .build()
        execute(req, String.serializer(), allowEmpty = true)
    }

    private fun <B> request(path: String, body: B, bodySer: KSerializer<B>): Request =
        Request.Builder()
            .url(joinUrl(baseUrl, path))
            .post(HubJson.encodeToString(bodySer, body).toRequestBody(JSON_MEDIA))
            .build()

    /**
     * 发请求 + 解析。
     *
     * 三件事必须做对：
     * 1. **在 IO 线程上跑** —— OkHttp 的同步调用会阻塞，落在主线程上是 ANR。
     * 2. **失败时先试着解析出结构化错误** —— hub 的 `{code,message,retryable}`
     *    是给人和 agent 读的，退化成"HTTP 409"等于把这份信息扔了。
     * 3. **网络异常和 HTTP 错误分开** —— 前者是"连不上"，后者是"它拒绝了你"，
     *    对用户来说是完全不同的两件事。
     */
    private suspend fun <T> execute(
        request: Request,
        serializer: KSerializer<T>,
        allowEmpty: Boolean = false,
    ): T = withContext(Dispatchers.IO) {
        val response = try {
            client.newCall(request).execute()
        } catch (e: IOException) {
            throw HubException.offline(e)
        }
        response.use {
            val text = it.body?.string().orEmpty()
            if (!it.isSuccessful) {
                val err = runCatching { HubJson.decodeFromString(ApiError.serializer(), text) }.getOrNull()
                throw HubException(
                    status = it.code,
                    error = err,
                    message = err?.message?.takeIf(String::isNotBlank)
                        ?: "请求失败（HTTP ${it.code}）",
                )
            }
            if (allowEmpty) {
                @Suppress("UNCHECKED_CAST")
                return@use text as T
            }
            // 解析失败通常意味着**打到了控制台的 index.html** —— 反向代理没把
            // 这条路径转给 hub。这句提示比 "Unexpected JSON token" 有用得多。
            try {
                HubJson.decodeFromString(serializer, text)
            } catch (e: Exception) {
                throw HubException(
                    status = it.code,
                    error = null,
                    message = "hub 回了一份读不懂的响应 —— 确认这个地址指向的是 agent-hub 本身",
                    cause = e,
                )
            }
        }
    }
}
