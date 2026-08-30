package org.agenthub.app

import android.app.Application
import org.agenthub.app.data.HubApi
import org.agenthub.app.data.Prefs
import org.agenthub.app.data.SessionCookieJar
import org.agenthub.app.data.newHttpClient
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking

/**
 * 依赖装配。**手写的容器，没上 DI 框架** —— 这个 app 一共三个依赖
 * （Prefs / cookie 罐 / HubApi），引一个框架的注解处理器要花的编译时间
 * 比它省下的代码多。真到了十几个依赖再说。
 */
class AgentHubApplication : Application() {

    lateinit var prefs: Prefs
        private set
    lateinit var api: HubApi
        private set
    lateinit var cookieJar: SessionCookieJar
        private set

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    override fun onCreate() {
        super.onCreate()
        prefs = Prefs(this)

        // 启动时把上次的会话和 hub 地址读回来。
        // **runBlocking 在这里是可以接受的**：读的是两个字符串，而代价是
        // 少掉一整个"还不知道自己登没登录"的中间状态 —— 那个状态会让首屏
        // 先闪一下登录页再跳走。第一帧还没画，阻塞几毫秒没人看得见。
        val (savedUrl, savedSession) = runBlocking {
            prefs.hubUrlNow() to prefs.sessionNow()
        }

        cookieJar = SessionCookieJar(initial = savedSession) { cookie ->
            // CookieJar 的回调不是挂起函数，所以落盘丢到自己的 scope 上。
            scope.launch { prefs.setSession(cookie) }
        }
        api = HubApi(newHttpClient(cookieJar), baseUrl = savedUrl.orEmpty())
    }
}
