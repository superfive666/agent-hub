package org.agenthub.app.data

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/**
 * 线上数据的形状。**唯一契约是 `docs/api/openapi.yaml`。**
 *
 * web 那边用 openapi-typescript 生成，这边是手写的 —— 所以**契约改了，
 * 这个文件要手动跟一遍**。这条负担明写在 ADR-0009 的「影响」里。
 *
 * 每个类都开了默认值：hub 加字段时旧版 app 不该崩。
 * `ignoreUnknownKeys` 在 [HubClient] 的 Json 配置里开着，方向反过来也成立。
 */

@Serializable
data class ApiError(
    val code: String = "",
    val message: String = "",
    val retryable: Boolean = false,
    val retryAfter: Int? = null,
)

@Serializable
data class AdminMe(
    val username: String = "",
    val authMode: String = "password",
    val timezone: String = "UTC",
)

@Serializable
data class AgentSummary(
    val agentId: String = "",
    val name: String = "",
    val summary: String? = null,
    val status: String? = null,
    val online: Boolean? = null,
    val tier: String? = null,
    val skills: List<String> = emptyList(),
    val limitations: List<String> = emptyList(),
    /** false = 还没写 Agent Card。控制台要靠它把两类分开展示 */
    val hasCard: Boolean = true,
)

@Serializable
data class DirectoryResponse(val agents: List<AgentSummary> = emptyList())

@Serializable
data class TodoSummary(
    val threadId: String = "",
    val title: String = "",
    val status: String? = null,
    val primaryAgentId: String? = null,
    val primaryAgentName: String? = null,
    val primaryAgentOnline: Boolean? = null,
    val replyCount: Int = 0,
    val startedAt: String? = null,
    val updatedAt: String? = null,
    val confirmedAt: String? = null,
)

@Serializable
data class TodosResponse(val todos: List<TodoSummary> = emptyList())

@Serializable
data class ThreadWatcher(
    val agentId: String = "",
    val name: String? = null,
    val online: Boolean? = null,
)

@Serializable
data class Post(
    val postId: String = "",
    val authorKind: String = "agent",
    val authorId: String? = null,
    val authorName: String? = null,
    val body: String = "",
    val createdAt: String? = null,
    val seq: Long? = null,
)

@Serializable
data class ThreadDetail(
    val threadId: String = "",
    val title: String = "",
    val kind: String = "todo",
    val status: String? = null,
    val primaryAgentId: String? = null,
    val primaryAgentName: String? = null,
    val confirmedAt: String? = null,
    val startedAt: String? = null,
    val watchers: List<ThreadWatcher> = emptyList(),
    val posts: List<Post> = emptyList(),
)

@Serializable
data class TodoStep(
    val stepId: String = "",
    val kind: String? = null,
    val status: String? = null,
    val title: String = "",
    val detail: String? = null,
    val createdAt: String? = null,
    val agentName: String? = null,
)

@Serializable
data class StepsResponse(val steps: List<TodoStep> = emptyList())

@Serializable
data class BoardItem(
    val threadId: String = "",
    val title: String = "",
    val kind: String = "todo",
    val status: String? = null,
    val primaryAgentName: String? = null,
    val at: String? = null,
    val posts: Int = 0,
)

@Serializable
data class BoardResponse(
    val date: String = "",
    val groupBy: String = "activity",
    val items: List<BoardItem> = emptyList(),
)

@Serializable
data class OnlineWindow(
    val longpoll: Int = 0,
    val webhook: Int = 0,
    val cron: Int = 0,
)

@Serializable
data class RateLimits(
    val tweetsPerHour: Int = 0,
    val inboxWritesPerMinute: Int = 0,
    val apiRequestsPerMinute: Int = 0,
)

@Serializable
data class Settings(
    val timezone: String = "UTC",
    val longPollMaxSeconds: Int = 30,
    val inboxRetentionDays: Int = 30,
    val onlineWindowSeconds: OnlineWindow = OnlineWindow(),
    val rateLimits: RateLimits = RateLimits(),
)

@Serializable
data class Health(
    val outboxLagSeconds: Double = 0.0,
    val outboxPending: Int = 0,
    val outboxDead: Int = 0,
    val workerAlive: Boolean = false,
    val pendingLongPolls: Int = 0,
)

@Serializable
data class LoginRequest(val username: String, val password: String)

@Serializable
data class CreateTodoRequest(
    val title: String,
    val body: String,
    /** **必选。** 一条 todo 必须有且只有一个主 agent，数据库层强制 */
    val primaryAgentId: String,
    val mentions: List<String> = emptyList(),
)

@Serializable
data class CreatePostRequest(
    val body: String,
    val mentions: List<String> = emptyList(),
)

@Serializable
data class TodoStateRequest(
    val status: String? = null,
    /** 用户确认闸门。放行是 true —— 这个动作只有人做得了 */
    val confirmed: Boolean? = null,
)

@Serializable
data class CreateAgentRequest(
    val name: String,
    val summary: String? = null,
    val issueToken: Boolean = true,
)

@Serializable
data class CreatedAgent(
    val agentId: String = "",
    val name: String = "",
    /** 明文注册 token。**只在这里出现一次**，之后就再也拿不到了 */
    val registrationToken: String? = null,
    val joinUrl: String? = null,
    val expiresAt: String? = null,
)

/**
 * 改简介 / 停用启用。
 *
 * **没有 name 字段，这是故意的。** 名字是 `@` 提及的唯一标识，改掉之后正文里
 * 已经写好的 `@old-name` 会**静默失效** —— 解析不到就当普通文本忽略，
 * 没有任何地方会报错，只是那些 agent 从此收不到本该属于它们的通知。
 *
 * 两个字段都可省：`enabled` 省略表示这次不动状态，
 * 只想改简介的请求不会顺手把 agent 停掉。
 */
@Serializable
data class UpdateAgentRequest(
    val purpose: String? = null,
    val enabled: Boolean? = null,
)

/**
 * 409 `agent_in_use` 的留痕计数。
 *
 * **这不是异常情况，是删除操作的正常结果之一** —— 界面要把它翻译成
 * 「这个 agent 有历史，改用停用」，而不是弹一句「删除失败」。
 * 拿到计数才说得出「它背着 3 条 todo」，只说「删不掉」等于让用户自己去猜。
 */
@Serializable
data class AgentRefs(
    val todos: Int = 0,
    val tweets: Int = 0,
    val steps: Int = 0,
)

@Serializable
data class ApkMeta(
    val available: Boolean = false,
    val version: String? = null,
    val filename: String? = null,
    val sizeBytes: Long? = null,
    @SerialName("updatedAt") val updatedAt: String? = null,
)
