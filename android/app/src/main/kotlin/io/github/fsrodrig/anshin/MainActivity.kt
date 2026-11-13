package io.github.fsrodrig.anshin

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.SystemBarStyle
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.graphics.toArgb

class MainActivity : ComponentActivity() {
    private val link by lazy { AgentLink(TailnetHttp.client) }
    private val themeSetting by lazy { ThemeSetting(applicationContext.settingsStore) }

    override fun onCreate(savedInstanceState: Bundle?) {
        styleSystemBars(Theme.Default.colors)
        super.onCreate(savedInstanceState)
        setContent {
            AnshinTheme(themeSetting) {
                val colors = AnshinTheme.colors
                LaunchedEffect(colors) { styleSystemBars(colors) }
                PulseScreen(link)
            }
        }
    }

    /**
     * Every theme is dark, so the bars keep light icons whatever the system's light or dark mode; the default
     * `auto` style would give them dark icons in system light mode. The scrim is the theme background because below
     * API 35 the system paints it behind the bars; from API 35 the app draws behind them itself.
     */
    private fun styleSystemBars(colors: AnshinColors) {
        val style = SystemBarStyle.dark(colors.background.toArgb())
        enableEdgeToEdge(statusBarStyle = style, navigationBarStyle = style)
    }
}
