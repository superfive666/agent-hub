package org.agenthub.app.data

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import org.agenthub.app.ui.theme.ThemeMode

private val Context.dataStore: DataStore<Preferences> by preferencesDataStore(name = "agent_hub")

/**
 * 本地存的**全部东西**就这三样：hub 地址、会话 cookie、主题偏好。
 *
 * 没有业务数据缓存 —— 这是刻意的（见立项书 §2「v1 不做什么」）：
 * 缓存一旦存在，「界面上看到的」和「hub 上真实的」就可能不一致，
 * 而这个平台的整个正确性模型建立在「以 hub 为准」上。
 *
 * ## 会话 cookie 存在哪
 *
 * 存在 DataStore 里，也就是应用私有目录。这是 Android 上的标准基线：
 * 别的应用读不到，root 过的设备读得到。
 *
 * **没有上 EncryptedSharedPreferences**，理由是那要额外背一个
 * `androidx.security` 的 alpha 依赖，而它挡的威胁（物理接触 + root）
 * 在这里同时也能拿到别的东西。清单里 `allowBackup=false` 才是这一块
 * 真正有用的那条 —— 它防的是会话被云备份**复制到另一台设备**。
 */
class Prefs(private val context: Context) {

    private object Keys {
        val hubUrl = stringPreferencesKey("hub_url")
        val session = stringPreferencesKey("session_cookie")
        val theme = stringPreferencesKey("theme_mode")
    }

    val hubUrl: Flow<String?> = context.dataStore.data.map { it[Keys.hubUrl] }
    val themeMode: Flow<ThemeMode> = context.dataStore.data.map {
        when (it[Keys.theme]) {
            "light" -> ThemeMode.Light
            "dark" -> ThemeMode.Dark
            else -> ThemeMode.System
        }
    }

    suspend fun hubUrlNow(): String? = context.dataStore.data.first()[Keys.hubUrl]
    suspend fun sessionNow(): String? = context.dataStore.data.first()[Keys.session]

    suspend fun setHubUrl(url: String) {
        context.dataStore.edit { it[Keys.hubUrl] = url }
    }

    suspend fun setSession(cookie: String?) {
        context.dataStore.edit {
            if (cookie == null) it.remove(Keys.session) else it[Keys.session] = cookie
        }
    }

    suspend fun setThemeMode(mode: ThemeMode) {
        context.dataStore.edit {
            it[Keys.theme] = when (mode) {
                ThemeMode.Light -> "light"
                ThemeMode.Dark -> "dark"
                ThemeMode.System -> "system"
            }
        }
    }

    /**
     * 换 hub 时必须**连会话一起清掉**。
     *
     * cookie 是签给某一台 hub 的（HMAC 里带着那台的 SESSION_SECRET），
     * 带着旧 cookie 去打新 hub 只会全程 401 —— 而用户会以为是密码错了，
     * 因为界面上看不出「你还揣着上一台的会话」。
     */
    suspend fun switchHub(url: String) {
        context.dataStore.edit {
            it[Keys.hubUrl] = url
            it.remove(Keys.session)
        }
    }
}
