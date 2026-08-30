package org.agenthub.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertIs
import kotlin.test.assertTrue

/**
 * 需求：app 要问用户「你的 hub 在哪」（网页上不存在这一步）。
 *
 * 规整错了的症状是**每个请求都 404**，而用户会以为是账号问题 ——
 * 所以这里的每条规则都值一个用例。
 */
class HubUrlTest {

    private fun ok(input: String): HubUrlResult.Ok {
        val r = normalizeHubUrl(input)
        assertIs<HubUrlResult.Ok>(r, "应当能规整：$input")
        return r
    }

    @Test
    fun `没写协议时补 https，不补 http`() {
        // 管理员口令要从这条路走。猜错方向的代价是明文发密码。
        assertEquals("https://hub.example.com", ok("hub.example.com").baseUrl)
    }

    @Test
    fun `尾斜杠和空白都吃掉`() {
        assertEquals("https://hub.example.com", ok("  https://hub.example.com/  ").baseUrl)
    }

    @Test
    fun `从浏览器地址栏整条复制过来也要能用`() {
        // 用户最可能干的事：在电脑上打开控制台，把地址栏整条发到手机上。
        for (path in listOf("/login", "/threads/abc-123", "/board", "/settings", "/download")) {
            assertEquals("https://hub.example.com", ok("https://hub.example.com$path").baseUrl, path)
        }
    }

    @Test
    fun `挂在子路径下的 hub 不会被剥掉路径`() {
        // 只剥已知的控制台路由。把 https://example.com/agent-hub 剥成 https://example.com
        // 会彻底坏掉，而且比不剥更难查 —— 用户会觉得地址明明填对了。
        assertEquals("https://example.com/agent-hub", ok("https://example.com/agent-hub").baseUrl)
    }

    @Test
    fun `端口保留 —— 局域网自建就是这个形态`() {
        assertEquals("http://192.168.1.5:8080", ok("http://192.168.1.5:8080").baseUrl)
    }

    @Test
    fun `明文 http 到公网地址不拦，但要标记出来`() {
        // 拦下来等于让一部分人（局域网、自签证书）根本用不了；
        // 但登录页要能显眼地说一句「你的口令会明文发出去」。
        assertTrue(ok("http://hub.example.com").insecure)
        assertTrue(!ok("https://hub.example.com").insecure)
    }

    @Test
    fun `局域网和本机的 http 不算不安全`() {
        // 这是这个平台的正常部署形态，报警会变成狼来了。
        for (h in listOf("http://localhost:8080", "http://127.0.0.1:8080", "http://192.168.1.5", "http://10.0.0.2")) {
            assertTrue(!ok(h).insecure, h)
        }
    }

    @Test
    fun `说不通的输入给一句能直接显示给用户的话`() {
        for (bad in listOf("", "   ", "ftp://hub.example.com", "https://")) {
            val r = normalizeHubUrl(bad)
            assertIs<HubUrlResult.Invalid>(r, "应当被拒：$bad")
            assertTrue(r.error.isNotBlank(), "错误信息不能是空的：$bad")
        }
    }

    @Test
    fun `拼路径不会拼出双斜杠`() {
        assertEquals("https://h.example.com/api/admin/me", joinUrl("https://h.example.com/", "/api/admin/me"))
        assertEquals("https://h.example.com/api/admin/me", joinUrl("https://h.example.com", "api/admin/me"))
    }
}
