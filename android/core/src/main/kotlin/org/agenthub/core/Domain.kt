package org.agenthub.core

/**
 * 领域模型。形状来自 `docs/api/openapi.yaml`，**不是从后端结构体抄的**。
 *
 * 这一层刻意不认识 HTTP、不认识 JSON、也不认识 Android ——
 * 它是纯 Kotlin，所以状态流转、日期切分这些**有需求可依的规则**
 * 能在没有 Android SDK 的机器上测（见 `settings.gradle.kts` 的说明）。
 */

/** 一条 todo 的状态。顺序就是推进顺序，`ordinal` 在进度条里被依赖。 */
enum class TodoStatus(val wire: String) {
    AwaitingResponse("awaiting_response"),
    Clarifying("clarifying"),
    InProgress("in_progress"),
    AwaitingReview("awaiting_review"),
    Done("done"),

    /**
     * 取消**不在推进顺序里** —— 它是从任何一步都能跳到的终点。
     * 放进 STATUS_FLOW 会让进度条上多出一个永远不会亮的格子。
     */
    Cancelled("cancelled");

    companion object {
        fun from(wire: String?): TodoStatus? = entries.firstOrNull { it.wire == wire }
    }
}

/**
 * 进度条上的推进顺序。**不含 Cancelled**，理由见上。
 *
 * 它同时兼着待办页筛选器的值域，所以往里塞一个不存在的状态，
 * 会平白多出一个永远筛不到东西的筛选项。
 */
val STATUS_FLOW: List<TodoStatus> = listOf(
    TodoStatus.AwaitingResponse,
    TodoStatus.Clarifying,
    TodoStatus.InProgress,
    TodoStatus.AwaitingReview,
    TodoStatus.Done,
)

enum class StepKind(val wire: String) {
    Clarification("clarification"),
    Plan("plan"),
    Progress("progress"),
    Blocked("blocked"),
    Deliverable("deliverable"),
    Confirmation("confirmation");

    companion object {
        fun from(wire: String?): StepKind? = entries.firstOrNull { it.wire == wire }
    }
}

enum class StepStatus(val wire: String) {
    Pending("pending"),
    InProgress("in_progress"),
    Done("done"),
    Blocked("blocked");

    companion object {
        fun from(wire: String?): StepStatus? = entries.firstOrNull { it.wire == wire }
    }
}

enum class AgentStatus(val wire: String) {
    /** 只是一条占位记录，还没拿注册 token 换过长期凭证 */
    PendingRegistration("pending_registration"),
    Active("active"),
    Disabled("disabled");

    companion object {
        fun from(wire: String?): AgentStatus? = entries.firstOrNull { it.wire == wire }
    }
}

/** 通知档位。决定「多久没动静算离线」，三档窗口差着一个数量级。 */
enum class Tier(val wire: String) {
    LongPoll("longpoll"),
    Webhook("webhook"),
    Cron("cron");

    companion object {
        fun from(wire: String?): Tier? = entries.firstOrNull { it.wire == wire }
    }
}

/** 发帖人是人还是 agent。设计语言 §1.1 的四重信号全挂在这一位上。 */
enum class AuthorKind { Admin, Agent }

/**
 * 一个 agent 在这条 thread 里的参与身份。
 *
 * 「被 @ 只产生关注关系，没有回复义务」—— 所以 Watcher 在界面上是虚线描边，
 * 那是**语义**不是装饰：虚线表示「这不是一个承诺」。
 */
enum class Participation { Primary, Watcher }
