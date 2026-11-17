package io.github.fsrodrig.anshin

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.Density
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onRoot
import kotlin.time.Duration.Companion.minutes
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

private const val MACHINE = "machine"

/** Every state of the Machine screen, in every theme. Goldens live in `src/test/screenshots/`, named `<state>.<theme>.png`. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class ScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    private fun capture(name: String, content: @Composable () -> Unit) {
        compose.setContent { AnshinTheme(theme, content) }
        compose.onRoot().captureFrame("$name.${theme.key}.png")
    }

    private fun pulse(name: String, view: MachineView) = capture("pulse-$name") { MachineScreen(view, MACHINE) }

    @Test fun connecting() = pulse("connecting", MachineView.Connecting)

    // The flat fields are the newest Sample's, as on the wire, so the numbers and the charts' last points agree.
    @Test fun live() = pulse("live", live(Pulse(schema = PULSE_SCHEMA, kind = PulseKind.Full, hostname = MACHINE, cpuPct = 21.4, cpuTempC = 52.1,
        memTotalBytes = 8589934592, memAvailableBytes = 6442450944, memUsedBytes = 2147483648, memUsedPct = 25.0,
        swapTotalBytes = 4294967296, swapUsedBytes = 0), WindowSamples.endingAt(cpuNow = 21.4, memoryNow = 25.0)))

    @Test fun liveWithoutCpu() =
        pulse("live-cpu-unavailable", live(Pulse(schema = PULSE_SCHEMA, kind = PulseKind.Full, hostname = null, cpuPct = null)))

    @Test fun unavailableAtLargeFontScale() = capture("pulse-live-unavailable-large-font") {
        val density = LocalDensity.current
        CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale = 1.3f)) {
            MachineScreen(live(Pulse(schema = PULSE_SCHEMA, kind = PulseKind.Full, hostname = null, cpuPct = null)), MACHINE)
        }
    }


    @Test fun forbidden() = pulse("forbidden", MachineView.Forbidden)

    @Test fun incompatible() = pulse("incompatible", MachineView.Incompatible)

    @Test fun noAnswer() = pulse("unreachable", unreachable())

    // MachineBook refuses a literal outside the tailnet, so only a name that resolves outside it reaches this state.
    @Test fun unreachableOutsideTailnet() = pulse("unreachable-outside-tailnet", unreachable(outsideTailnet = true))

    @Test fun agentCannotReachTailscaled() = pulse("failed-503", unreachable(httpStatus = 503))

    @Test fun otherHttpFailure() = pulse("failed-500", unreachable(httpStatus = 500))

    // A crit Verdict, so the screen shows it is quoted as history rather than current.
    private val earlier = Pulse(schema = PULSE_SCHEMA, kind = PulseKind.Full, hostname = MACHINE, cpuPct = 21.4, cpuTempC = 52.1,
        memUsedPct = 25.0, swapTotalBytes = 4294967296, swapUsedBytes = 0, battery = BatteryReport.NoBattery,
        verdict = Verdict(VerdictLevel.Crit, listOf(Reason("cpu_hot", VerdictLevel.Crit, "CPU at 97.0 °C (crit at 95.0 °C)"))))

    @Test fun staleData() = pulse("stale", stale(earlier, WindowSamples.endingAt(cpuNow = 21.4, memoryNow = 25.0)))

    @Test fun unreachableWithTheLastData() =
        pulse("unreachable-last", unreachable(last = received(earlier, WindowSamples.endingAt(cpuNow = 21.4, memoryNow = 25.0), age = 3.minutes)))

    @Test fun unreachableWithTheVpnHint() = pulse("unreachable-vpn-hint", unreachable(vpnHint = true))


    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
