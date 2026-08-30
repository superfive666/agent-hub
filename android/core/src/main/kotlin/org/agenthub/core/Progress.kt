package org.agenthub.core

/** 进度条上一格的状态。 */
enum class StepState { Done, Current, Todo }

data class ProgressStep(val label: String, val state: StepState)

/** 闸门节点在进度条上的名字。卡片标题也用它，两处叫法必须一致。 */
const val GATE_LABEL: String = "需求确认"

/**
 * 进度条。
 *
 * **为什么确认是画出来的、而不是加进 [STATUS_FLOW]。** 确认不是一个 status，
 * 它是 `confirmedAt` 这一位数据（ADR-0008 明确拒绝了「加一个 awaiting_approval 状态」）。
 * [STATUS_FLOW] 还兼着待办页筛选器的值域，往里塞一个不存在的状态，
 * 会平白多出一个永远筛不到东西的筛选项。
 *
 * 所以流程本身不动，只在「澄清中」和「进行中」之间**插一个由 confirmedAt 推出来的
 * 节点**：确认过是 ✓，没确认就是灰的下一步。它必须出现在进度里 —— 未确认的 todo
 * 卡在这儿推不动，进度条上却完全看不出原因，人只会以为 agent 在偷懒。
 *
 * @param withGate 不画闸门的调用方（tweet、以及不关心闸门的地方）传 false。
 *   默认跟着 [confirmedAt] 走：显式传了这一位，就说明调用方关心闸门。
 */
fun progressOf(
    status: TodoStatus?,
    confirmedAt: String? = null,
    withGate: Boolean = true,
): List<ProgressStep> {
    val at = status?.let { STATUS_FLOW.indexOf(it) } ?: -1
    val steps = STATUS_FLOW.mapIndexed { i, s ->
        ProgressStep(
            label = statusLabel(s),
            state = when {
                at < 0 -> StepState.Todo
                i < at -> StepState.Done
                i == at -> StepState.Current
                else -> StepState.Todo
            },
        )
    }
    if (!withGate) return steps

    val gateAt = STATUS_FLOW.indexOf(TodoStatus.InProgress)
    val gate = ProgressStep(
        label = GATE_LABEL,
        state = if (confirmedAt != null) StepState.Done else StepState.Todo,
    )
    return steps.take(gateAt) + gate + steps.drop(gateAt)
}

/**
 * 这条 todo 现在是不是卡在确认闸门上。
 *
 * 「卡住」的判定不是「没确认」—— 一条刚建出来、还在待响应的 todo 也没确认，
 * 但它没被卡住，它只是还没走到那儿。**真正卡住的是「澄清完了、等人点头」**，
 * 这时候界面上必须明说，否则人只会以为 agent 在偷懒。
 */
fun isAwaitingConfirmation(status: TodoStatus?, confirmedAt: String?): Boolean =
    confirmedAt == null && (status == TodoStatus.Clarifying || status == TodoStatus.AwaitingResponse)
