package org.agenthub.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

class TextTest {

    @Test
    fun `头像缩写：拉丁取两位大写，CJK 取一个字`() {
        assertEquals("NO", initialsOf("nova"))
        assertEquals("RO", initialsOf("Robin"))
        assertEquals("信", initialsOf("信使"))
        assertEquals("ノ", initialsOf("ノヴァ"))
        assertEquals("??", initialsOf(null))
        assertEquals("??", initialsOf("   "))
    }

    @Test
    fun `邮箱打码保留完整域名`() {
        // 用户要靠域名认出自己登的是哪个账号，打掉域名等于没法确认。
        assertEquals("wuchao**@gmail.com", maskEmail("wuchao900726@gmail.com"))
    }

    @Test
    fun `短邮箱和普通用户名原样返回`() {
        // 口令模式下这是普通用户名，打码只会让人以为自己登错了。
        assertEquals("superfive", maskEmail("superfive"))
        assertEquals("ab@x.com", maskEmail("ab@x.com"))
        assertEquals("@leading", maskEmail("@leading"))
        assertEquals("trailing@", maskEmail("trailing@"))
    }

    @Test
    fun `@ 提及按名字精确收边`() {
        // 收边写错的代价是**发通知给一个无关的 agent**，而发出去就收不回来。
        val agents = listOf("a-1" to "nova", "a-2" to "nova2", "a-3" to "kilo")
        assertEquals(listOf("a-1"), mentionedAgentIds("@nova 看一下", agents))
        assertEquals(listOf("a-2"), mentionedAgentIds("@nova2 看一下", agents))
        assertEquals(listOf("a-1", "a-3"), mentionedAgentIds("@nova @kilo", agents))
    }

    @Test
    fun `匹配不上的 @ 直接忽略，不报错`() {
        // 正文里本来就可能有普通的 @（邮箱、口语）。
        val agents = listOf("a-1" to "nova")
        assertEquals(emptyList(), mentionedAgentIds("发到 me@example.com", agents))
        assertEquals(emptyList(), mentionedAgentIds("@nobody 在吗", agents))
    }

    @Test
    fun `@ 必须在行首或空白之后`() {
        val agents = listOf("a-1" to "nova")
        assertEquals(emptyList(), mentionedAgentIds("email:x@nova", agents))
    }

    @Test
    fun `名字里的正则元字符不会被当成模式`() {
        val agents = listOf("a-1" to "no.va")
        assertEquals(listOf("a-1"), mentionedAgentIds("@no.va", agents))
        // 转义漏了的话 `.` 会匹配任意字符，@noXva 也会中
        assertEquals(emptyList(), mentionedAgentIds("@noXva", agents))
    }

    @Test
    fun `刚敲下 @ 时返回空串，不是 null`() {
        // 空串能前缀匹配任何名字，下拉立刻把全部 agent 摊开 ——
        // 这正是 @ 提及的基本盘。返回 null 就变成「要先猜对首字母才有提示」。
        assertEquals("", mentionQueryAt("回复 @", 4))
        assertEquals("no", mentionQueryAt("回复 @no", 6))
        assertNull(mentionQueryAt("回复内容", 4))
        // 光标不在 @ 那一段末尾时不算
        assertNull(mentionQueryAt("@nova 已经打完了", 12))
    }
}
