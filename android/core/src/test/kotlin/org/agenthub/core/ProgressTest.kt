package org.agenthub.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * 需求：todo 的确认闸门（ADR-0008）。
 *
 * 「未确认的 todo 卡在这儿推不动，进度条上却完全看不出原因，
 * 人只会以为 agent 在偷懒」—— 所以闸门必须画出来，且必须画在正确的位置。
 */
class ProgressTest {

    @Test
    fun `闸门插在澄清中和进行中之间`() {
        val labels = progressOf(TodoStatus.Clarifying, confirmedAt = null).map { it.label }
        assertEquals(
            listOf("待响应", "澄清中", GATE_LABEL, "进行中", "待确认", "已完成"),
            labels,
        )
    }

    @Test
    fun `确认过的 todo 闸门是完成态`() {
        val gate = progressOf(TodoStatus.InProgress, confirmedAt = "2026-08-30T02:00:00Z")
            .first { it.label == GATE_LABEL }
        assertEquals(StepState.Done, gate.state)
    }

    @Test
    fun `没确认时闸门是灰的下一步，不是当前步`() {
        // 关键区分：闸门不抢「当前」这个位置 —— 当前步仍然是 status 指向的那一格，
        // 闸门只是横在路上的一个没走过的节点。两个都画成 current 的话，
        // 进度条上会同时有两个高亮，人读不出来到底走到哪了。
        val steps = progressOf(TodoStatus.Clarifying, confirmedAt = null)
        assertEquals(StepState.Todo, steps.first { it.label == GATE_LABEL }.state)
        assertEquals(StepState.Current, steps.first { it.label == "澄清中" }.state)
        assertEquals(1, steps.count { it.state == StepState.Current })
    }

    @Test
    fun `不关心闸门的调用方拿到的还是原来那五步`() {
        // tweet 没有闸门。多画一格出来，等于告诉用户「这条广播也要人点头」。
        val labels = progressOf(TodoStatus.InProgress, withGate = false).map { it.label }
        assertEquals(listOf("待响应", "澄清中", "进行中", "待确认", "已完成"), labels)
    }

    @Test
    fun `取消不在推进顺序里，进度条整条都是未走过`() {
        // Cancelled 是从任何一步都能跳到的终点，不是第六步。
        // 把它放进 STATUS_FLOW 会让进度条多出一个永远不会亮的格子。
        assertFalse(TodoStatus.Cancelled in STATUS_FLOW)
        val steps = progressOf(TodoStatus.Cancelled, withGate = false)
        assertTrue(steps.all { it.state == StepState.Todo })
    }

    @Test
    fun `状态为空时不假装走到了第一步`() {
        val steps = progressOf(null, withGate = false)
        assertTrue(steps.all { it.state == StepState.Todo })
    }

    @Test
    fun `卡在闸门上的判定`() {
        // 「卡住」不等于「没确认」：刚建出来的 todo 也没确认，但它没被卡住。
        val cases = listOf(
            Triple(TodoStatus.Clarifying, null, true),
            Triple(TodoStatus.AwaitingResponse, null, true),
            Triple(TodoStatus.Clarifying, "2026-08-30T02:00:00Z", false),
            // 已经在干了，说明闸门早过了 —— 这里返回 true 会让界面骗人
            Triple(TodoStatus.InProgress, null, false),
            Triple(TodoStatus.Done, null, false),
        )
        for ((status, confirmedAt, want) in cases) {
            assertEquals(want, isAwaitingConfirmation(status, confirmedAt), "status=$status confirmed=$confirmedAt")
        }
    }
}
