package io.github.fsrodrig.anshin

import androidx.activity.ComponentActivity
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.toPixelMap
import androidx.compose.ui.test.captureToImage
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import kotlin.math.abs

private const val MACHINE = "machine"

/** The header mark is the active theme's accent, and nothing a Machine reports changes it. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class AnshinMarkTest(private val theme: Theme) {
    @get:Rule val compose = createAndroidComposeRule<ComponentActivity>()

    private val machine = Pulse(schema = PULSE_SCHEMA, kind = PulseKind.Full, hostname = MACHINE, cpuPct = 21.0)
    private val crit = machine.copy(verdict = Verdict(VerdictLevel.Crit, listOf(Reason("memory_low", VerdictLevel.Crit, "Memory available is 3 %"))))
    private val states: List<Pair<String, MachineView>> = listOf(
        "Live ok" to live(machine.copy(verdict = Verdict(VerdictLevel.Ok, emptyList()))),
        "Live crit" to live(crit),
        "Stale crit" to stale(crit),
        "Unreachable" to unreachable(),
        "Connecting" to MachineView.Connecting,
    )

    private fun markPixels(): List<Color> {
        // captureToImage redraws only the ComposeView, and the API 37 NATIVE renderer can lose the window's own
        // display list once earlier tests' graphics objects are collected, leaving a frame of the bare window
        // background. Redrawing the whole window first makes the capture the screen as it is now.
        compose.runOnUiThread { compose.activity.window.decorView.invalidate() }
        compose.waitForIdle()
        val pixels = compose.onNodeWithTag(ANSHIN_MARK_TAG).captureToImage().toPixelMap()
        return (0 until pixels.height).flatMap { y -> (0 until pixels.width).map { x -> pixels[x, y] } }
    }

    @Test
    fun theHeaderMarkIsDrawnInTheActiveThemesAccent() {
        compose.setContent { AnshinTheme(theme) { MachineScreen {} } }
        val colors = theme.colors
        val pixels = markPixels()
        assertTrue("${theme.key}: no pixel is exactly the accent", pixels.count { it == colors.accent } > 20)
        // Antialiased edges blend the accent into the background, so every pixel must lie between the two.
        val strays = pixels.filterNot { between(it, colors.background, colors.accent) }.distinct()
        assertEquals("${theme.key}: pixels that are neither accent nor background", emptyList<Color>(), strays)
    }

    @Test
    fun theHeaderMarkNeverChangesWithTheVerdictOrConnectionState() {
        val current = mutableStateOf(states.first().second)
        compose.setContent {
            AnshinTheme(theme) {
                MachineScreen(topLine(listOf(WatchedMachine(MACHINE, current.value)))) {
                    PulseView(current.value, MACHINE)
                }
            }
        }
        val drawn = states.map { (name, result) ->
            compose.runOnIdle { current.value = result }
            compose.waitForIdle()
            name to markPixels()
        }
        val (firstName, first) = drawn.first()
        for ((name, pixels) in drawn.drop(1)) {
            assertEquals("${theme.key}: the mark in $name differs from $firstName", first, pixels)
        }
    }

    private fun between(c: Color, from: Color, to: Color): Boolean {
        val tolerance = 2.5f / 255
        fun t(a: Float, b: Float, v: Float) = if (abs(b - a) < tolerance) null else (v - a) / (b - a)
        val ts = listOfNotNull(t(from.red, to.red, c.red), t(from.green, to.green, c.green), t(from.blue, to.blue, c.blue))
        val share = ts.average().toFloat().coerceIn(0f, 1f)
        fun near(a: Float, b: Float, v: Float) = abs(a + (b - a) * share - v) <= tolerance
        return near(from.red, to.red, c.red) && near(from.green, to.green, c.green) && near(from.blue, to.blue, c.blue)
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
