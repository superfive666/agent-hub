package org.agenthub.core

/**
 * 枚举 → 中文。**英文枚举直接甩给用户等于没写。**
 *
 * 这份表和 `web/src/lib/format.ts` 里的那几张必须一字不差 ——
 * 同一个状态在网页上叫「待确认」、在 app 里叫「等待审核」的话，
 * 用户会以为那是两种不同的状态。
 */

fun statusLabel(status: TodoStatus?): String = when (status) {
    TodoStatus.AwaitingResponse -> "待响应"
    TodoStatus.Clarifying -> "澄清中"
    TodoStatus.InProgress -> "进行中"
    TodoStatus.AwaitingReview -> "待确认"
    TodoStatus.Done -> "已完成"
    TodoStatus.Cancelled -> "已取消"
    null -> "—"
}

/** 契约里冒出没见过的状态时**原样显示**，不要显示「—」—— 那会看起来像没有状态。 */
fun statusLabel(wire: String?): String =
    TodoStatus.from(wire)?.let { statusLabel(it) } ?: wire ?: "—"

fun stepKindLabel(kind: StepKind?): String = when (kind) {
    StepKind.Clarification -> "澄清"
    StepKind.Plan -> "计划"
    StepKind.Progress -> "进展"
    StepKind.Blocked -> "受阻"
    StepKind.Deliverable -> "交付物"
    StepKind.Confirmation -> "确认放行"
    null -> "—"
}

fun stepKindLabel(wire: String?): String =
    StepKind.from(wire)?.let { stepKindLabel(it) } ?: wire ?: "—"

fun stepStatusLabel(status: StepStatus?): String = when (status) {
    StepStatus.Pending -> "待开始"
    StepStatus.InProgress -> "进行中"
    StepStatus.Done -> "已完成"
    // 和 kind 的「受阻」区分开：这里说的是这一步现在被卡住了
    StepStatus.Blocked -> "卡住了"
    null -> "—"
}

fun stepStatusLabel(wire: String?): String =
    StepStatus.from(wire)?.let { stepStatusLabel(it) } ?: wire ?: "—"

fun agentStatusLabel(status: AgentStatus?): String = when (status) {
    AgentStatus.PendingRegistration -> "未接入"
    AgentStatus.Active -> "已接入"
    AgentStatus.Disabled -> "已停用"
    null -> "—"
}

fun agentStatusLabel(wire: String?): String =
    AgentStatus.from(wire)?.let { agentStatusLabel(it) } ?: wire ?: "—"

fun tierLabel(tier: Tier?): String = when (tier) {
    Tier.LongPoll -> "长轮询"
    Tier.Webhook -> "webhook"
    Tier.Cron -> "cron"
    null -> "—"
}

fun tierLabel(wire: String?): String = Tier.from(wire)?.let { tierLabel(it) } ?: wire ?: "—"

/**
 * 延迟的人话。**分段是刻意的**：agent 的响应延迟跨三个数量级
 * （长轮询几秒、webhook 几十秒、cron 半小时），全都写成「1834 秒」
 * 等于让人自己心算。
 */
fun latencyLabel(seconds: Int?): String = when {
    seconds == null -> "—"
    seconds < 90 -> "~$seconds 秒"
    seconds < 3600 -> "~${Math.round(seconds / 60.0)} 分钟"
    else -> "~${Math.round(seconds / 3600.0)} 小时"
}
