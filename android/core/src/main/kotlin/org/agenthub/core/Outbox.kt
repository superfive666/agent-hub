package org.agenthub.core

/**
 * outbox 健康度。设计语言 §1.4 —— **这条告警不可折叠、不可降级、不可挪到二级页面。**
 *
 * worker 挂掉是**完全静默**的失败：帖子照发、inbox 照拉，只是没有新东西。
 * `outbox_lag` 是唯一能发现它的地方。把判定放在 core 里、而不是散在几个
 * Composable 里，是为了让「什么时候必须出声」只有一个答案。
 */

/** 横幅出现的门槛，秒。低于它且 worker 活着 = 安静。 */
const val OUTBOX_BANNER_THRESHOLD_SECONDS: Double = 60.0

/** 设置页上那条「告警阈值」的展示值，秒。比横幅门槛更早出声。 */
const val OUTBOX_ALERT_THRESHOLD_SECONDS: Double = 30.0

sealed interface OutboxState {
    /** 一切正常，不画横幅。**这是唯一允许安静的分支。** */
    data object Quiet : OutboxState

    /** 滞后了，但 worker 还在。 */
    data class Lagging(val lagSeconds: Double, val pending: Int?) : OutboxState

    /** worker 不在了。这是最严重的一档 —— 从现在起没有任何新事件会进 inbox。 */
    data class WorkerDown(val lagSeconds: Double?, val pending: Int?) : OutboxState

    /**
     * 连健康状态都问不到。
     *
     * **这一档必须出声，不能当成"暂时没数据"静默掉** —— 那说明我们连滞后多少
     * 都不知道，等于把唯一的探针也关了。静默的代价正是 §1.4 要防的那件事。
     */
    data object Unknown : OutboxState
}

/**
 * @param healthy 拿到健康数据了吗。false = 查询失败/超时，走 [OutboxState.Unknown]。
 * @param workerAlive worker 还在不在。
 * @param lagSeconds `now() - min(occurred_at) WHERE status='pending'`。
 */
fun outboxStateOf(
    healthy: Boolean,
    workerAlive: Boolean,
    lagSeconds: Double?,
    pending: Int? = null,
): OutboxState {
    if (!healthy) return OutboxState.Unknown
    val lag = lagSeconds ?: 0.0
    if (!workerAlive) return OutboxState.WorkerDown(lagSeconds, pending)
    if (lag >= OUTBOX_BANNER_THRESHOLD_SECONDS) return OutboxState.Lagging(lag, pending)
    return OutboxState.Quiet
}

/** 横幅上的一句话。要说清楚**现在正在发生什么**，不是"出错了"。 */
fun outboxMessage(state: OutboxState): String = when (state) {
    OutboxState.Quiet -> ""
    OutboxState.Unknown ->
        "拿不到 outbox 状态 —— 现在连滞后多少都不知道。这台 hub 可能正常，也可能已经不再投递事件了。"
    is OutboxState.WorkerDown ->
        "投递 worker 不在了。帖子照发、inbox 照拉，但**不会再有新事件进来** —— " +
            "这是一个完全静默的失败，只有这里看得见。"
    is OutboxState.Lagging ->
        "outbox 滞后 ${latencyLabel(state.lagSeconds.toInt())}" +
            (state.pending?.let { "，${it} 条待投递" } ?: "") +
            "。agent 会晚收到消息，但不会丢。"
}
