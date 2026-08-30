package org.agenthub.app.data

import okhttp3.Cookie
import okhttp3.HttpUrl.Companion.toHttpUrl
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * 需求：app 用**和控制台完全相同的会话** —— 一个 `HttpOnly` Cookie，
 * 后端一个端点都不用改（ADR-0009）。
 *
 * 这几条盯的是那个 cookie 罐。它错了的症状全都是「登录成功之后每个请求都 401」，
 * 而那看起来完全像是后端的问题。
 */
class SessionCookieJarTest {

    private val url = "https://hub.example.com/api/admin/me".toHttpUrl()

    private fun cookie(name: String, value: String, expiresAt: Long = Long.MAX_VALUE) =
        Cookie.Builder()
            .name(name)
            .value(value)
            .domain("hub.example.com")
            .path("/")
            .expiresAt(expiresAt)
            .build()

    @Test
    fun `收下会话 cookie 并在后续请求里带上`() {
        val jar = SessionCookieJar()
        jar.saveFromResponse(url, listOf(cookie(SESSION_COOKIE, "signed.value")))

        val sent = jar.loadForRequest(url)
        assertEquals(1, sent.size)
        assertEquals(SESSION_COOKIE, sent[0].name)
        assertEquals("signed.value", sent[0].value)
    }

    @Test
    fun `别的 cookie 一律丢掉`() {
        // 反向代理、CDN、WAF 都可能种自己的 cookie。原样收下来的话它们会
        // 跟着每个请求发回去，而我们既不需要也不该替它们保管。
        val jar = SessionCookieJar()
        jar.saveFromResponse(
            url,
            listOf(cookie("__cf_bm", "x"), cookie("AWSALB", "y")),
        )
        assertTrue(jar.loadForRequest(url).isEmpty())
        assertNull(jar.session)
    }

    @Test
    fun `退出登录时后端下发的删除 cookie 要被当成删除`() {
        // 后端的 logout 下发的是一个空值、MaxAge<0 的 cookie —— 那是"删除"，
        // 不是"把会话设成空串"。不区分的话，退出之后 app 会揣着一个空会话
        // 反复 401，而界面上看起来像是"登录状态还在但坏了"。
        val jar = SessionCookieJar(initial = "old.session")
        jar.saveFromResponse(url, listOf(cookie(SESSION_COOKIE, "", expiresAt = 0L)))

        assertNull(jar.session)
        assertTrue(jar.loadForRequest(url).isEmpty())
    }

    @Test
    fun `没有会话时不发任何 cookie`() {
        assertTrue(SessionCookieJar().loadForRequest(url).isEmpty())
    }

    @Test
    fun `cookie 变了要通知上层落盘，没变就不通知`() {
        // 每收一个响应就写一次 DataStore 是无谓的磁盘写；更要紧的是
        // 少了这个回调，杀掉进程再打开就要重新登录一次。
        val seen = mutableListOf<String?>()
        val jar = SessionCookieJar(onChange = { seen.add(it) })

        jar.saveFromResponse(url, listOf(cookie(SESSION_COOKIE, "a")))
        jar.saveFromResponse(url, listOf(cookie(SESSION_COOKIE, "a"))) // 同值，不该再通知
        jar.saveFromResponse(url, listOf(cookie(SESSION_COOKIE, "b")))

        assertEquals(listOf("a", "b"), seen)
    }

    @Test
    fun `cookie 跟着请求的域名走`() {
        // 换 hub 之后 baseUrl 变了，cookie 必须种到新域名上，否则 OkHttp
        // 会因为域名不匹配而不发它 —— 表现同样是"每个请求都 401"。
        val jar = SessionCookieJar(initial = "s")
        val other = "https://other.example.com/api/admin/me".toHttpUrl()
        assertEquals("other.example.com", jar.loadForRequest(other).single().domain)
    }
}
