package io.github.fsrodrig.anshin

import android.net.ConnectivityManager
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.platform.LocalConfiguration

class MainActivity : ComponentActivity() {
    // One link for the Watcher and the test connection, over the app's only HTTP client.
    private val link by lazy { OkHttpAgentLink(TailnetHttp.client) }
    private val watcher by lazy {
        val connectivity = getSystemService(ConnectivityManager::class.java)
        Watcher(link, ElapsedRealtime, vpnLooksOff = { connectivity.vpnLooksOff() })
    }
    private val book by lazy { MachineBook(applicationContext.machineStore, link) }
    private val themeSetting by lazy { ThemeSetting(applicationContext.settingsStore) }

    override fun onCreate(savedInstanceState: Bundle?) {
        styleSystemBars(Theme.Default)
        super.onCreate(savedInstanceState)
        setContent {
            AnshinTheme(themeSetting) {
                val theme = AnshinTheme.theme
                // AndroidX restores the initial bar styles when the decor view receives a configuration change.
                LaunchedEffect(theme, LocalConfiguration.current) { styleSystemBars(theme) }
                AnshinApp(book, watcher, themeSetting)
            }
        }
    }

    /**
     * The bar icons follow the theme's appearance, not the system's light or dark mode, which the default `auto`
     * style would follow. The scrim is the theme background because below API 35 the system paints it behind the bars;
     * from API 35 the app draws behind them itself.
     */
    private fun styleSystemBars(theme: Theme) {
        val scrim = theme.colors.background.toArgb()
        val style = when (theme.appearance) {
            Appearance.Dark -> SystemBarStyle.dark(scrim)
            Appearance.Light -> SystemBarStyle.light(scrim, scrim)
        }
        enableEdgeToEdge(statusBarStyle = style, navigationBarStyle = style)
    }
}
