package io.github.fsrodrig.anshin

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.Density
import com.github.takahirom.roborazzi.captureRoboImage
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

private const val MACHINE = "machine"

@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class MachineDetailScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    private fun pulse(name: String, status: @Composable () -> Unit) {
        compose.setContent { AnshinTheme(theme) { PulseLayout(MACHINE, onInputChange = {}, onWatch = {}, status = status) } }
        compose.onRoot().captureRoboImage("pulse-$name.${theme.key}.png")
    }

    private val detailPulse = Pulse(
        schema = 1, kind = PulseKind.Full, hostname = MACHINE, cpuPct = 21.4,
        memTotalBytes = 8589934592, memAvailableBytes = 6442450944, memUsedBytes = 2147483648, memUsedPct = 25.0,
        swapTotalBytes = 4294967296, swapUsedBytes = 0,
        swapEntries = listOf(SwapEntry("/dev/zram0", true, 2147483648, 0), SwapEntry("/swapfile", false, 2147483648, 0)),
        load1 = 0.0, load5 = 1.25, load15 = 2.5, uptimeS = 12345.67,
        distro = "Example Linux with a long distribution name", kernel = "6.12-example-kernel", arch = "aarch64", agentVersion = "test-build",
    )

    @Test fun detailScrollAndBackAtLargeFontScale() {
        compose.setContent {
            AnshinTheme(theme) {
                val density = LocalDensity.current
                CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale = 1.3f)) {
                    PulseLayout(MACHINE, onInputChange = {}, onWatch = {}) { PulseView(PulseResult.Ok(detailPulse), MACHINE) }
                }
            }
        }
        compose.onNodeWithText("Machine details").performScrollTo().performClick()
        compose.onNodeWithText("Memory used").performScrollTo().assertIsDisplayed()
        compose.onRoot().captureRoboImage("detail-memory-large-font.${theme.key}.png")
        compose.onNodeWithText("zram").performScrollTo().assertIsDisplayed()
        compose.onRoot().captureRoboImage("detail-zram-large-font.${theme.key}.png")
        compose.onNodeWithText("Load 1 min").performScrollTo().assertIsDisplayed()
        compose.onRoot().captureRoboImage("detail-load-large-font.${theme.key}.png")
        compose.onNodeWithText("test-build").performScrollTo().assertIsDisplayed()
        compose.onRoot().captureRoboImage("detail-info-large-font.${theme.key}.png")
        compose.onAllNodesWithText("Back to Machine")[1].performScrollTo().performClick()
        compose.onNodeWithText("Live").performScrollTo().assertIsDisplayed()
    }

    @Test fun detailUnavailable() = pulse("detail-unavailable") { MachineDetail(Pulse(schema = 1), onBack = {}) }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
