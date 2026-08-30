package org.agenthub.core

/**
 * hub 地址的规整。
 *
 * **这是 app 独有的一块，网页上不存在** —— 网页天然知道自己从哪来，
 * app 不知道，得问用户。而用户会输入的东西五花八门：
 * `hub.example.com`、`https://hub.example.com/`、
 * 甚至从浏览器地址栏里整条复制过来的 `https://hub.example.com/threads/abc`。
 *
 * 全都要能用。规整错了的症状是**每个请求都 404**，而用户会以为是账号问题。
 */

/** 规整的结果。失败时 [error] 是一句能直接显示给用户的话。 */
sealed interface HubUrlResult {
    data class Ok(val baseUrl: String, val insecure: Boolean) : HubUrlResult
    data class Invalid(val error: String) : HubUrlResult
}

/**
 * 控制台自己的路由。用户从地址栏复制过来时最可能带上这些，
 * 直接当成 base 的话每个 API 请求都会多出一截路径。
 *
 * **只剥这些已知的**，别的路径原样保留 —— hub 是可以挂在子路径下的，
 * 把 `https://example.com/agent-hub` 剥成 `https://example.com` 就彻底坏了，
 * 而且坏得比不剥更难查。
 */
private val CONSOLE_PATHS = setOf(
    "login", "threads", "board", "todos", "directory", "settings", "download", "api",
)

fun normalizeHubUrl(input: String?): HubUrlResult {
    val raw = input?.trim().orEmpty()
    if (raw.isEmpty()) return HubUrlResult.Invalid("填一个 hub 地址，比如 hub.example.com")

    // 没写协议的按 https 补。**补 https 不补 http** —— 管理员口令要从这条路走，
    // 猜错方向的代价是明文发密码。真要用 http 的（局域网、自签证书）
    // 得自己把 http:// 打出来，那时他是知情的。
    val withScheme = if (raw.contains("://")) raw else "https://$raw"

    val url = runCatching { java.net.URI(withScheme) }.getOrNull()
        ?: return HubUrlResult.Invalid("这个地址看不懂，检查一下有没有多余的空格或符号")

    val scheme = url.scheme?.lowercase()
    if (scheme != "http" && scheme != "https") {
        return HubUrlResult.Invalid("只支持 http/https，收到的是 $scheme")
    }
    val host = url.host
    if (host.isNullOrBlank()) {
        return HubUrlResult.Invalid("看不出主机名，形如 hub.example.com")
    }

    val port = if (url.port > 0) ":${url.port}" else ""

    // 路径：去掉尾斜杠，再剥掉一段已知的控制台路由（只剥一层，且只剥已知的）
    var path = (url.path ?: "").trimEnd('/')
    val segments = path.split('/').filter { it.isNotEmpty() }
    if (segments.isNotEmpty() && segments.first().lowercase() in CONSOLE_PATHS) {
        // 首段就是控制台路由 → 整条路径都是控制台的，base 是域名根
        path = ""
    }

    val base = "$scheme://$host$port$path"
    // http 到非本地地址：**不拦，只标记**。局域网自建 hub 是这个平台的正常形态，
    // 拦下来等于让一部分人根本用不了；但登录页要能显眼地说一句
    // 「你的口令会明文发出去」，那是用户该知道的事。
    val localish = host == "localhost" || host.startsWith("127.") || host.startsWith("10.") ||
        host.startsWith("192.168.") || host == "::1"
    return HubUrlResult.Ok(baseUrl = base, insecure = scheme == "http" && !localish)
}

/** 把 base 和一条 API 路径拼起来。两边的斜杠都规整过，不会拼出 `//api`。 */
fun joinUrl(baseUrl: String, path: String): String =
    baseUrl.trimEnd('/') + "/" + path.trimStart('/')
