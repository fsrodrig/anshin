package io.github.fsrodrig.anshin

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Density
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onRoot
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

private const val MACHINE = "machine"

/** Every state of every screen, in every theme. Goldens live in `src/test/screenshots/`, named `<state>.<theme>.png`. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class ScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    private fun capture(name: String, content: @Composable () -> Unit) {
        compose.setContent { AnshinTheme(theme, content) }
        compose.onRoot().captureFrame("$name.${theme.key}.png")
    }

    private fun pulse(name: String, input: String = MACHINE, status: @Composable () -> Unit) =
        capture("pulse-$name") { PulseLayout(input, onInputChange = {}, onWatch = {}, status = status) }

    @Test fun idle() = pulse("idle", input = "") {}

    @Test fun notAMachineAddress() = pulse("not-a-machine-address", input = "$MACHINE:99999") { NotAMachineAddress() }

    @Test fun connecting() = pulse("connecting") { PulseView(null, MACHINE) }

    // The flat fields are the newest Sample's, as on the wire, so the numbers and the charts' last points agree.
    @Test fun live() = pulse("live") { PulseView(PulseResult.Ok(Pulse(schema = PULSE_SCHEMA, kind = PulseKind.Full, hostname = MACHINE, cpuPct = 21.4, cpuTempC = 52.1,
        memTotalBytes = 8589934592, memAvailableBytes = 6442450944, memUsedBytes = 2147483648, memUsedPct = 25.0,
        swapTotalBytes = 4294967296, swapUsedBytes = 0)), MACHINE, WindowSamples.endingAt(cpuNow = 21.4, memoryNow = 25.0)) }

    @Test fun liveWithoutCpu() =
        pulse("live-cpu-unavailable") { PulseView(PulseResult.Ok(Pulse(schema = PULSE_SCHEMA, kind = PulseKind.Full, hostname = null, cpuPct = null)), MACHINE) }

    @Test fun unavailableAtLargeFontScale() = capture("pulse-live-unavailable-large-font") {
        val density = LocalDensity.current
        CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale = 1.3f)) {
            PulseLayout(MACHINE, onInputChange = {}, onWatch = {}) {
                PulseView(PulseResult.Ok(Pulse(schema = PULSE_SCHEMA, kind = PulseKind.Full, hostname = null, cpuPct = null)), MACHINE)
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


    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
