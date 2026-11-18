package io.github.fsrodrig.anshin

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performScrollToIndex
import androidx.compose.ui.test.performScrollToKey
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.unit.Density
import kotlin.time.Duration.Companion.minutes
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Each detail state's top frame in every theme; the frames further down, and the large font, in the default theme
 * only, because the detail is taller than the 640 dp frame.
 */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class MachineDetailScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    private val notCurrent = NOT_CURRENT

    private fun show(state: MachineDetailState, fontScale: Float = 1f) {
        compose.setContent {
            AnshinTheme(theme) {
                val density = LocalDensity.current
                CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale)) {
                    MachineScreen(state)
                }
            }
        }
    }

    private fun frames(name: String, everyTheme: List<String> = emptyList(), defaultOnly: List<String> = emptyList()) {
        compose.onNodeWithTag(SCREEN_CONTENT_TAG).performScrollToIndex(0)
        compose.onRoot().captureFrame("detail-$name-overview.${theme.key}.png")
        for (stop in everyTheme + if (theme == Theme.Default) defaultOnly else emptyList()) {
            compose.onNodeWithTag(SCREEN_CONTENT_TAG).performScrollToKey(stop)
            compose.onNodeWithTag(stop).assertIsDisplayed()
            compose.onRoot().captureFrame("detail-$name-$stop.${theme.key}.png")
        }
    }

    private fun assertText(text: String) {
        compose.onNodeWithText(text, substring = true).assertIsDisplayed()
    }

    private fun assertLastVerdict() {
        compose.onNodeWithText("ok, 3\u00A0min\u00A0ago").assertExists()
        compose.onNodeWithText("ok").assertDoesNotExist()
    }

    @Test fun live() {
        show(DetailSamples.state(Connection.Live))
        assertText("Live")
        assertText("last Sample 2\u00A0s\u00A0ago")
        compose.onNodeWithText("ok").assertIsDisplayed()
        frames("live", everyTheme = listOf("cpu"))
        if (theme == Theme.Default) {
            val list = compose.onNodeWithTag(SCREEN_CONTENT_TAG)
            list.performScrollToIndex(0)
            var frame = 0
            while (true) {
                compose.onRoot().captureFrame("detail-live-frame-${frame++}.${theme.key}.png")
                val range = list.fetchSemanticsNode().config[SemanticsProperties.VerticalScrollAxisRange]
                if (range.value() >= range.maxValue()) break
                list.performSemanticsAction(SemanticsActions.ScrollBy) { scroll ->
                    scroll(0f, list.fetchSemanticsNode().size.height * 0.85f)
                }
                compose.waitForIdle()
                check(frame < 100) { "Live detail did not reach its end" }
            }
        }
    }

    @Test fun stale() {
        show(DetailSamples.state(Connection.Stale, age = 3.minutes))
        assertText("Stale")
        assertText("last Sample 3\u00A0min\u00A0ago")
        assertText(notCurrent)
        assertLastVerdict()
        frames("stale", defaultOnly = listOf("cpu", "storage", "network"))
    }

    @Test fun unreachable() {
        show(DetailSamples.state(DetailSamples.unreachable(), age = 3.minutes))
        assertText("Unreachable")
        assertText("last Sample 3\u00A0min\u00A0ago")
        assertText("No Pulse from machine. The Machine may be off, its Agent stopped, or this phone off the tailnet.")
        assertText("Next try in 4\u00A0s")
        assertText(notCurrent)
        assertLastVerdict()
        frames("unreachable", defaultOnly = listOf("cpu", "storage", "network"))
    }

    @Test fun unreachableOutsideTailnet() {
        if (theme != Theme.Default) return
        show(DetailSamples.state(DetailSamples.unreachable(outsideTailnet = true), age = 3.minutes, machine = "192.0.2.1"))
        assertText("192.0.2.1 isn't a tailnet address, so Anshin won't connect to it.")
        assertLastVerdict()
        compose.onRoot().captureFrame("detail-unreachable-outside-tailnet-overview.${theme.key}.png")
    }

    @Test fun unreachableAfterAnHttpFailureWithTheVpnHint() {
        if (theme != Theme.Default) return
        show(DetailSamples.state(DetailSamples.unreachable(vpnHint = true, httpStatus = 503), age = 3.minutes))
        assertText("Unreachable")
        assertText("The Agent on machine can't ask tailscaled who this phone is.")
        assertText("Next try in 4\u00A0s")
        assertText("VPN looks off")
        assertText(notCurrent)
        compose.onRoot().captureFrame("detail-unreachable-503-vpn-hint-overview.${theme.key}.png")
        compose.onNodeWithTag(SCREEN_CONTENT_TAG).performScrollToKey("verdict")
        assertLastVerdict()
        compose.onRoot().captureFrame("detail-unreachable-503-vpn-hint-verdict.${theme.key}.png")
    }

    @Test fun everySectionUnavailable() {
        show(DetailSamples.state(Connection.Live, pulse = Pulse(schema = PULSE_SCHEMA), window = null))
        assertText("Verdict unavailable")
        frames("unavailable", defaultOnly = listOf("network"))
    }

    @Test fun liveWithGap() {
        if (theme != Theme.Default) return
        val held = WindowSamples.withGap()
        val pulse = Pulse(schema = PULSE_SCHEMA, kind = PulseKind.Full, hostname = DetailSamples.MACHINE,
            cpuPct = held.window.cpuPct!!.last(), memUsedPct = held.window.memUsedPct!!.last())
        show(DetailSamples.state(Connection.Live, pulse = pulse, window = held))
        compose.onRoot().captureFrame("detail-gap-overview.${theme.key}.png")
        compose.onNodeWithTag(SCREEN_CONTENT_TAG).performScrollToKey("window")
        assertText("last 30 min, 1 Gap of 5 min")
        compose.onRoot().captureFrame("detail-gap-window.${theme.key}.png")
        compose.onNodeWithTag(SCREEN_CONTENT_TAG).performScrollToKey("cpu")
        compose.onRoot().captureFrame("detail-gap-cpu.${theme.key}.png")
    }

    @Test fun liveAtLargeFont() {
        if (theme != Theme.Default) return
        show(DetailSamples.state(Connection.Live), fontScale = 1.3f)
        assertText("last Sample 2\u00A0s\u00A0ago")
        frames("large-font", defaultOnly = listOf("cpu", "storage", "network"))
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
