package io.github.fsrodrig.anshin

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge

class MainActivity : ComponentActivity() {
    private val link by lazy { AgentLink(TailnetHttp.client) }
    private val themeSetting by lazy { ThemeSetting(applicationContext.settingsStore) }

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        setContent {
            AnshinTheme(themeSetting) {
                PulseScreen(link)
            }
        }
    }
}
