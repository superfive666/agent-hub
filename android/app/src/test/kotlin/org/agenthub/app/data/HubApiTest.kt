package org.agenthub.app.data

import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

/**
 * 需求：app 打的是**和控制台完全相同的 `/api/admin/` 那一套**，
 * 错误要能被界面翻译成一句人话（不是甩一个状态码给用户）。
 */
class HubApiTest {

    private lateinit var server: MockWebServer
    private lateinit var jar: SessionCookieJar
    private lateinit var api: HubApi

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
        jar = SessionCookieJar()
        api = HubApi(newHttpClient(jar), server.url("/").toString().trimEnd('/'))
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun `登录成功后会话被收下，后续请求自动带上`() = runTest {
        server.enqueue(
            MockResponse()
                .setResponseCode(204)
                .addHeader("Set-Cookie", "$SESSION_COOKIE=signed.value; Path=/; HttpOnly"),
        )
        server.enqueue(MockResponse().setBody("""{"username":"superfive","authMode":"password","timezone":"Asia/Singapore"}"""))

        api.login("superfive", "pw")
        val me = api.me()

        assertEquals("superfive", me.username)
        server.takeRequest() // login
        val second = server.takeRequest()
        assertTrue(
            "第二发必须带上会话 cookie，否则登录等于白登",
            second.getHeader("Cookie")?.contains("$SESSION_COOKIE=signed.value") == true,
        )
    }

    @Test
    fun `打的是 admin 路由，不是 agent 路由`() = runTest {
        // app 是给那一个人类管理员用的。agent 侧那套 Bearer 凭证
        // 不该出现在这个 app 里（立项书 §2）。
        server.enqueue(MockResponse().setBody("""{"todos":[]}"""))
        api.todos()
        assertEquals("/api/admin/todos", server.takeRequest().path)
    }

    @Test
    fun `后端的结构化错误被原样带出来，不退化成状态码`() = runTest {
        server.enqueue(
            MockResponse().setResponseCode(409).setBody(
                """{"code":"todo_not_confirmed","message":"这条 todo 还没被确认放行","retryable":false}""",
            ),
        )
        try {
            api.setTodoState("t-1", TodoStateRequest(status = "in_progress"))
            fail("应当抛 HubException")
        } catch (e: HubException) {
            assertEquals(409, e.status)
            assertEquals("todo_not_confirmed", e.error?.code)
            // 界面直接显示这句话。换成 "HTTP 409" 的话，用户不知道该做什么。
            assertEquals("这条 todo 还没被确认放行", e.message)
        }
    }

    @Test
    fun `401 要能被认出来，好把人送回登录页`() = runTest {
        server.enqueue(MockResponse().setResponseCode(401).setBody("""{"code":"unauthorized","message":"没有会话"}"""))
        try {
            api.me()
            fail("应当抛 HubException")
        } catch (e: HubException) {
            assertTrue(e.unauthorized)
        }
    }

    @Test
    fun `拿到 HTML 时说的是地址问题，不是解析错误`() = runTest {
        // 这是最常见的部署错误：反向代理没把这条路径转给 hub，
        // 请求被静态站接走，返回控制台的 index.html。
        // 报 "Unexpected JSON token" 会让人去查代码，而该查的是代理配置。
        server.enqueue(MockResponse().setBody("<!doctype html><html><body>console</body></html>"))
        try {
            api.me()
            fail("应当抛 HubException")
        } catch (e: HubException) {
            assertTrue("提示要指向地址/代理，实得：${e.message}", e.message.contains("agent-hub"))
        }
    }

    @Test
    fun `连不上时给的是能读懂的一句话`() = runTest {
        server.shutdown()
        try {
            api.me()
            fail("应当抛 HubException")
        } catch (e: HubException) {
            assertEquals(0, e.status)
            assertTrue(e.message.contains("连不上"))
        }
    }

    @Test
    fun `契约里多出来的字段不会让解析炸掉`() = runTest {
        // hub 加了字段而 app 还没更新时，不忽略未知键的话**每个响应都解析失败**，
        // 界面全空 —— 而后端看起来一切正常。
        server.enqueue(
            MockResponse().setBody(
                """{"username":"superfive","authMode":"password","timezone":"UTC","brandNewField":123}""",
            ),
        )
        assertEquals("superfive", api.me().username)
    }

    @Test
    fun `下载元信息是公开端点，路径不带 api 前缀`() = runTest {
        // 用的是对外那个正式地址。/api/download/meta 只是留给还没改过
        // 反向代理的部署的同义词，客户端不该主动用它。
        server.enqueue(MockResponse().setBody("""{"available":true,"version":"0.1.0","sizeBytes":13107200}"""))
        val meta = api.apkMeta()
        assertEquals("/download/meta", server.takeRequest().path)
        assertTrue(meta.available)
        assertEquals("0.1.0", meta.version)
    }
}
