package org.agenthub.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * 需求：设计语言 §1.1 —— 人和 agent 必须一眼分得开，四重信号少一重都不行。
 *
 * 这几条用例盯的是**数据层能不能撑起那四重信号**。画得对不对要看真机，
 * 但如果 `isHuman` / `displayName` / `participation` 这三样给错了，
 * 画得再对也是错的。
 */
class AuthorTest {

    @Test
    fun `人类的名字不带前缀，agent 一律带 @`() {
        val human = authorOf(AuthorKind.Admin, null, "superfive")
        val agent = authorOf(AuthorKind.Agent, "a-1", "nova")
        assertEquals("superfive", human.displayName)
        assertEquals("@nova", agent.displayName)
    }

    @Test
    fun `isHuman 这一位就是四重信号的开关`() {
        assertTrue(authorOf(AuthorKind.Admin, null, "superfive").isHuman)
        assertTrue(!authorOf(AuthorKind.Agent, "a-1", "nova").isHuman)
    }

    @Test
    fun `主 agent 与关注者分得开`() {
        // §1.2：一条 todo 有且只有一个主 agent。主 agent 带辉光和「主 agent」chip，
        // 关注者是虚线描边 —— 虚线在这里是语义：被 @ 只产生关注关系，没有回复义务。
        val primary = authorOf(AuthorKind.Agent, "a-1", "nova", primaryAgentId = "a-1")
        val watcher = authorOf(AuthorKind.Agent, "a-2", "kilo", primaryAgentId = "a-1")
        assertEquals(Participation.Primary, primary.participation)
        assertEquals(Participation.Watcher, watcher.participation)
    }

    @Test
    fun `人类没有参与身份 —— 他不是这条 todo 的主 agent 也不是关注者`() {
        // 给人类挂上 Watcher 的话，他的气泡会带上虚线描边，
        // 而虚线的语义是「没有回复义务」—— 对唯一的那个人来说这句话毫无意义。
        assertNull(authorOf(AuthorKind.Admin, null, "superfive").participation)
    }

    @Test
    fun `名字缺失时给一个能读的兜底，不是空白`() {
        assertEquals("管理员", authorOf(AuthorKind.Admin, null, null).name)
        assertEquals("@agent", authorOf(AuthorKind.Agent, "a-1", "  ").displayName)
    }

    @Test
    fun `在线状态只从关注者名单里取`() {
        val online = authorOf(AuthorKind.Agent, "a-1", "nova", "a-1", mapOf("a-1" to true))
        val unknown = authorOf(AuthorKind.Agent, "a-9", "zeta", "a-1", mapOf("a-1" to true))
        assertEquals(true, online.online)
        // 名单里没有 = 不知道，**不是离线**。画成离线是在陈述一件我们并不知道的事。
        assertNull(unknown.online)
    }
}
