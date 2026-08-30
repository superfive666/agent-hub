package org.agenthub.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertTrue

/**
 * 需求：`outbox_lag` 告警不可关闭，也不能因为"太吵"降级（设计语言 §1.4 / ADR-0004）。
 *
 * worker 挂掉是**完全静默**的失败 —— 帖子照发、inbox 照拉，只是没有新东西。
 * 这几条用例盯的是「什么时候必须出声」，因为那是唯一能发现它的地方。
 */
class OutboxTest {

    @Test
    fun `一切正常时安静`() {
        assertEquals(OutboxState.Quiet, outboxStateOf(healthy = true, workerAlive = true, lagSeconds = 0.4))
    }

    @Test
    fun `worker 不在了必须出声，哪怕滞后是零`() {
        // 刚挂掉的那一刻 lag 还是 0 —— 只看 lag 的话会安静地漏掉整个故障。
        val s = outboxStateOf(healthy = true, workerAlive = false, lagSeconds = 0.0)
        assertIs<OutboxState.WorkerDown>(s)
        assertTrue(outboxMessage(s).isNotBlank())
    }

    @Test
    fun `拿不到健康数据时也要出声`() {
        // 那说明我们连滞后多少都不知道，静默掉等于把唯一的探针也关了。
        val s = outboxStateOf(healthy = false, workerAlive = true, lagSeconds = null)
        assertEquals(OutboxState.Unknown, s)
        assertTrue(outboxMessage(s).isNotBlank())
    }

    @Test
    fun `滞后超过门槛就出声`() {
        assertEquals(OutboxState.Quiet, outboxStateOf(true, true, OUTBOX_BANNER_THRESHOLD_SECONDS - 1))
        assertIs<OutboxState.Lagging>(outboxStateOf(true, true, OUTBOX_BANNER_THRESHOLD_SECONDS))
        assertIs<OutboxState.Lagging>(outboxStateOf(true, true, 900.0))
    }

    @Test
    fun `告警文案说的是现在正在发生什么，不是出错了`() {
        val down = outboxMessage(outboxStateOf(true, workerAlive = false, lagSeconds = 900.0))
        // 「帖子照发、inbox 照拉，但不会再有新事件」—— 这句话是这条告警的全部价值，
        // 换成「系统异常」的话，看到的人不知道该做什么，也不知道现在损失了什么。
        assertTrue(down.contains("不会再有新事件"), down)

        val lag = outboxMessage(outboxStateOf(true, true, 900.0, pending = 12))
        assertTrue(lag.contains("12 条"), lag)
        // 滞后不丢事件 —— 这一点要说，否则人会以为消息没了
        assertTrue(lag.contains("不会丢"), lag)
    }
}
