package org.agenthub.app

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import org.agenthub.app.nav.AppNav
import org.agenthub.app.ui.theme.AgentHubTheme

class MainActivity : ComponentActivity() {

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // 舞台要铺满整块屏 —— 状态栏和导航栏下面透出来的必须是那片色雾，
        // 而不是一条系统灰。安全区由各页面自己 padding 出来。
        enableEdgeToEdge()

        val app = application as AgentHubApplication

        setContent {
            val vm: AppViewModel = viewModel(
                factory = AppViewModel.Factory(app.api, app.prefs, app.cookieJar),
            )
            val mode by vm.themeMode.collectAsStateWithLifecycle()
            AgentHubTheme(mode = mode) {
                AppNav(vm)
            }
        }
    }
}
