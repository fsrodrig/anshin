package io.github.fsrodrig.anshin

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Density
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

private const val MACHINE = "machine"

/** Every state of every screen, in every theme. Goldens live in `src/test/screenshots/<theme>/`. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class ScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    private fun capture(name: String, content: @Composable () -> Unit) {
        compose.setContent { AnshinTheme(theme, content) }
        compose.onRoot().captureRoboImage("$name.${theme.key}.png")
    }

    private fun pulse(name: String, input: String = MACHINE, status: @Composable () -> Unit) =
        capture("pulse-$name") { PulseLayout(input, onInputChange = {}, onWatch = {}, status = status) }

    @Test fun idle() = pulse("idle", input = "") {}

    @Test fun notAMachineAddress() = pulse("not-a-machine-address", input = "$MACHINE:99999") { NotAMachineAddress() }

    @Test fun connecting() = pulse("connecting") { PulseView(null, MACHINE) }

    @Test fun live() = pulse("live") { PulseView(PulseResult.Ok(Pulse(PULSE_SCHEMA, MACHINE, 21.4)), MACHINE) }

    @Test fun liveWithoutCpu() =
        pulse("live-cpu-unavailable") { PulseView(PulseResult.Ok(Pulse(PULSE_SCHEMA, null, null)), MACHINE) }

    @Test fun unavailableAtLargeFontScale() = capture("pulse-live-unavailable-large-font") {
        val density = LocalDensity.current
        CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale = 1.3f)) {
            PulseLayout(MACHINE, onInputChange = {}, onWatch = {}) {
                PulseView(PulseResult.Ok(Pulse(PULSE_SCHEMA, null, null)), MACHINE)
            }
        }
    }

    @Test fun forbidden() = pulse("forbidden") { PulseView(PulseResult.Forbidden, MACHINE) }

    @Test fun incompatible() = pulse("incompatible") { PulseView(PulseResult.Incompatible, MACHINE) }

    @Test fun unreachable() =
        pulse("unreachable") { PulseView(PulseResult.Unreachable(outsideTailnet = false), MACHINE) }

    @Test fun unreachableOutsideTailnet() = pulse("unreachable-outside-tailnet", input = "192.0.2.1") {
        PulseView(PulseResult.Unreachable(outsideTailnet = true), "192.0.2.1")
    }

    @Test fun agentCannotReachTailscaled() = pulse("failed-503") { PulseView(PulseResult.Failed(503), MACHINE) }

    @Test fun otherHttpFailure() = pulse("failed-500") { PulseView(PulseResult.Failed(500), MACHINE) }

    @Test fun tokens() = capture("tokens") { TokenSheet(theme) }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}

/**
 * Not a screen: every token of a theme, with the accent beside each Verdict color and their CIEDE2000 distance and
 * hue difference, so a reviewer can see they stay apart before any screen shows a Verdict.
 */
@Composable
private fun TokenSheet(theme: Theme) {
    val c = theme.colors
    val mono = AnshinTheme.type.mono
    Column(
        Modifier.fillMaxSize().background(c.background).padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text(theme.label, color = c.text, fontSize = 18.sp)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            listOf("Live" to c.accent, "ok" to c.ok, "warn" to c.warn, "crit" to c.crit).forEach { (word, color) ->
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Box(Modifier.size(56.dp).background(color, RoundedCornerShape(4.dp)))
                    Text(word, color = color, style = mono, fontSize = 14.sp)
                    if (color != c.accent) {
                        Text(
                            "ΔE %.0f %.0f°".format(ColorMath.deltaE2000(c.accent, color), ColorMath.hueDifference(c.accent, color)),
                            color = c.muted,
                            style = mono,
                            fontSize = 10.sp,
                        )
                    }
                }
            }
        }
        Column(
            Modifier.fillMaxWidth().background(c.surface, RoundedCornerShape(8.dp))
                .border(1.dp, c.border, RoundedCornerShape(8.dp)).padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            c.byRole().forEach { (role, color) ->
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Box(Modifier.size(width = 32.dp, height = 16.dp).background(color).border(1.dp, c.muted))
                    Text("%-12s#%06X".format(role, color.toArgb() and 0xFFFFFF), color = c.text, style = mono, fontSize = 11.sp)
                    Text("42%", color = color, style = mono, fontSize = 11.sp)
                }
            }
        }
    }
}
