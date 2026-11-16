package io.github.fsrodrig.anshin

import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performSemanticsAction
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Settings and the licences list from top to bottom, and the font's licence, in every theme. The version is the
 * "Development build" a build without a release number shows, so no golden holds a version number.
 */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class SettingsScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    /** One frame per screenful, each scrolled by most of a screen, until the end. */
    private fun frames(name: String) {
        val scroller = compose.onNode(hasScrollAction())
        var frame = 0
        while (true) {
            compose.onRoot().captureFrame("$name-$frame.${theme.key}.png")
            frame++
            val range = scroller.fetchSemanticsNode().config[SemanticsProperties.VerticalScrollAxisRange]
            if (range.value() >= range.maxValue()) break
            scroller.performSemanticsAction(SemanticsActions.ScrollBy) { scroll ->
                scroll(0f, scroller.fetchSemanticsNode().size.height * 0.85f)
            }
            compose.waitForIdle()
            check(frame < 20) { "$name did not reach its end" }
        }
    }

    @Test fun settings() {
        compose.setContent {
            AnshinTheme(theme) { Settings(listOf(Machines.laptop, Machines.desktop), theme, version = null, SettingsActions()) }
        }
        frames("settings")
    }

    @Test fun licences() {
        compose.setContent { AnshinTheme(theme) { Licences(onBack = {}, onOpen = {}) } }
        frames("licences")
    }

    @Test fun fontLicence() {
        compose.setContent { AnshinTheme(theme) { LicenceText(openSourceLicences.single { it.font == MONO_FONT }, onBack = {}) } }
        compose.onRoot().captureFrame("licence-font.${theme.key}.png")
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
