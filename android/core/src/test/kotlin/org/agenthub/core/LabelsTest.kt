package org.agenthub.core

import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * 需求：英文枚举直接甩给用户等于没写。
 *
 * 这份表和 `web/src/lib/format.ts` 必须一字不差 —— 同一个状态在网页上叫
 * 「待确认」、在 app 里叫「等待审核」的话，用户会以为那是两种不同的状态。
 */
class LabelsTest {

    @Test
    fun `状态的中文和 web 一字不差`() {
        val want = mapOf(
            "awaiting_response" to "待响应",
            "clarifying" to "澄清中",
            "in_progress" to "进行中",
            "awaiting_review" to "待确认",
            "done" to "已完成",
            "cancelled" to "已取消",
        )
        for ((wire, label) in want) assertEquals(label, statusLabel(wire), wire)
    }

    @Test
    fun `契约里冒出没见过的值时原样显示，不显示破折号`() {
        // "—" 看起来像「这条没有状态」，而真实情况是「有个我们还不认识的状态」。
        assertEquals("archived", statusLabel("archived"))
        assertEquals("—", statusLabel(null as String?))
    }

    @Test
    fun `步骤类型与步骤状态的中文`() {
        assertEquals("确认放行", stepKindLabel("confirmation"))
        assertEquals("交付物", stepKindLabel("deliverable"))
        // kind 的「受阻」和 status 的「卡住了」要分得开：前者说这一步是关于阻塞的，
        // 后者说这一步现在推不动。写成同一个词，看板上就读不出区别。
        assertEquals("受阻", stepKindLabel("blocked"))
        assertEquals("卡住了", stepStatusLabel("blocked"))
    }

    @Test
    fun `未接入和已停用要分得开`() {
        // 「未接入」= 只是一条占位记录，还没换过凭证；「已停用」= 换过但被关掉了。
        // 混成一个词的话，运维分不清该发注册 token 还是该去启用。
        assertEquals("未接入", agentStatusLabel("pending_registration"))
        assertEquals("已停用", agentStatusLabel("disabled"))
        assertEquals("已接入", agentStatusLabel("active"))
    }

    @Test
    fun `延迟按数量级分段，不甩秒数`() {
        assertEquals("~30 秒", latencyLabel(30))
        assertEquals("~2 分钟", latencyLabel(120))
        assertEquals("~30 分钟", latencyLabel(1800))
        assertEquals("~1 小时", latencyLabel(3600))
        assertEquals("—", latencyLabel(null))
    }

    @Test
    fun `档位的中文`() {
        assertEquals("长轮询", tierLabel("longpoll"))
        assertEquals("webhook", tierLabel("webhook"))
        assertEquals("cron", tierLabel("cron"))
    }
}
