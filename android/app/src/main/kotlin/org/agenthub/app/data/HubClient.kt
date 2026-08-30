package org.agenthub.app.data

import kotlinx.serialization.json.Json
import okhttp3.Cookie
import okhttp3.CookieJar
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit

/** hub 的会话 cookie 名。和后端 `sessionCookie` 常量一致，改一边就全断。 */
const val SESSION_COOKIE = "hub_session"

/**
 * 会话 cookie 罐。
 *
 * ## 为什么是 cookie 而不是 token
 *
 * hub 的会话就是一个 `HttpOnly` Cookie（HMAC 签名，12 小时），
 * 控制台和 app 用的是**同一套鉴权，一个端点都不用改**（ADR-0009）。
 * 给 app 单发一种 token 意味着后端要多一条鉴权路径 ——
 * 多一条就多一处可能配错、可能忘记吊销的地方，为的只是省下这个类。
 *
 * ## 只认这一个 cookie
 *
 * 别的 cookie 一律丢掉。反向代理、CDN、WAF 都可能种自己的 cookie，
 * 原样收下来的话它们会跟着每个请求发回去，而我们既不需要也不该替它们保管。
 *
 * ## 线程安全
 *
 * OkHttp 在任意线程上调 [loadForRequest]/[saveFromResponse]。
 * 用 `@Volatile` 的单值就够 —— 只有一个会话，不需要一张表。
 */
class SessionCookieJar(
    initial: String? = null,
    /** cookie 变了就回调，由上层落盘。**不在这里直接写 DataStore** —— 那是挂起函数，而这两个方法不是。 */
    private val onChange: (String?) -> Unit = {},
) : CookieJar {

    @Volatile
    var session: String? = initial
        private set

    override fun loadForRequest(url: HttpUrl): List<Cookie> {
        val v = session ?: return emptyList()
        return listOf(
            Cookie.Builder()
                .name(SESSION_COOKIE)
                .value(v)
                .domain(url.host)
                .path("/")
                .build(),
        )
    }

    override fun saveFromResponse(url: HttpUrl, cookies: List<Cookie>) {
        val c = cookies.firstOrNull { it.name == SESSION_COOKIE } ?: return
        // 退出登录时后端下发的是一个 MaxAge<0 的空值 cookie —— 那是"删除"，
        // 不是"设置成空串"。不区分的话，退出后 app 会揣着一个空会话反复 401。
        val next = if (c.value.isBlank() || c.expiresAt < System.currentTimeMillis()) null else c.value
        if (next != session) {
            session = next
            onChange(next)
        }
    }

    fun set(value: String?) {
        session = value
        onChange(value)
    }
}

/**
 * 契约的 JSON 解析。
 *
 * `ignoreUnknownKeys` 必须开：hub 加了字段而 app 还没更新时，
 * 不开的话**每个响应都解析失败**，界面全空 —— 而后端看起来一切正常。
 */
val HubJson = Json {
    ignoreUnknownKeys = true
    isLenient = true
    explicitNulls = false
    encodeDefaults = true
}

/**
 * 建一个 OkHttp 客户端。
 *
 * 超时给得比默认宽：hub 可能就在一条家用宽带后面，而 agent 的长轮询
 * 最长挂 30 秒 —— 读超时短于它的话，任何一次长轮询都会在事件到达前被掐断。
 * v1 还没用长轮询，但这个数留着，免得将来加的时候忘了这一条。
 */
fun newHttpClient(cookieJar: CookieJar): OkHttpClient =
    OkHttpClient.Builder()
        .cookieJar(cookieJar)
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(60, TimeUnit.SECONDS)
        .writeTimeout(30, TimeUnit.SECONDS)
        .retryOnConnectionFailure(true)
        .build()
