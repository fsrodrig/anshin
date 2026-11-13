package io.github.fsrodrig.anshin

import androidx.compose.runtime.SideEffect
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.text.rememberTextMeasurer
import androidx.datastore.preferences.core.PreferenceDataStoreFactory
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.cancel
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

@RunWith(RobolectricTestRunner::class)
class ThemeSwitchTest {
    @get:Rule val compose = createComposeRule()
    @get:Rule val folder = TemporaryFolder()

    private val job = Job()
    private val setting by lazy {
        ThemeSetting(
            PreferenceDataStoreFactory.create(
                scope = CoroutineScope(Dispatchers.IO + job),
                produceFile = { folder.root.resolve("settings.preferences_pb") },
            ),
        )
    }

    @After
    fun closeStore() = job.cancel()

    @Test
    fun changingTheStoredThemeRecomposesWithItsTokensWithoutARestart() {
        var drawn: AnshinColors? = null
        compose.setContent {
            AnshinTheme(setting) {
                val colors = AnshinTheme.colors
                SideEffect { drawn = colors }
                PulseScreen(AgentLink(OkHttpClient()))
            }
        }
        compose.waitUntil { drawn == Theme.InkAndBone.colors }
        compose.onNode(hasSetTextAction()).performTextInput("machine")

        runBlocking { setting.select(Theme.MatteBlack) }

        compose.waitUntil { drawn == Theme.MatteBlack.colors }
        // The screen kept its state, so it was recomposed, not recreated.
        compose.onNode(hasSetTextAction()).assertTextContains("machine")
    }

    @Test
    fun monoTextIsJetBrainsMonoWithTabularDigits() {
        var widths = emptyList<Int>()
        compose.setContent {
            AnshinTheme(Theme.InkAndBone) {
                val measurer = rememberTextMeasurer()
                val style = AnshinTheme.type.mono
                val results: List<TextLayoutResult> = listOf("1111", "8888", "iiii", "WWWW").map { measurer.measure(it, style) }
                SideEffect { widths = results.map { it.size.width } }
            }
        }
        compose.waitForIdle()
        assertEquals(1, widths.distinct().size)
    }
}
