package io.github.fsrodrig.anshin

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.unit.Density
import com.github.takahirom.roborazzi.captureRoboImage
import java.io.File
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class BatteryScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    // Every other card value is available, so each "— unavailable" on the card is the battery's.
    private val machine = Pulse(
        schema = 1, hostname = "machine", cpuPct = 21.0, memUsedPct = 40.0, swapTotalBytes = 2048, swapUsedBytes = 0,
        filesystems = listOf(Filesystem("0:35", "/", "btrfs", 4096, 1024, 3072, 25.0, 0.0)), primaryDisk = "0:35",
    )
    private val discharging = BatteryReport.Present(Battery(64.0, BatteryStatus.Discharging, 7.5, 9000.0,
        listOf(BatteryRow("BAT0", 64.0, BatteryStatus.Discharging, 7.5, 87.5))))
    private val charging = BatteryReport.Present(Battery(64.0, BatteryStatus.Charging, 15.0, null,
        listOf(BatteryRow("BAT0", 64.0, BatteryStatus.Charging, 15.0, 87.5))))

    @Test fun cardShowsChargeAndState() {
        val current = mutableStateOf(machine.copy(battery = discharging))
        compose.setContent { AnshinTheme(theme) {
            PulseLayout("machine", onInputChange = {}, onWatch = {}) { PulseView(PulseResult.Ok(current.value), "machine") }
        } }
        compose.onNodeWithText("64%").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("discharging").performScrollTo().assertIsDisplayed()
        compose.onRoot().captureRoboImage("battery-discharging.${theme.key}.png")

        compose.runOnIdle { current.value = machine.copy(battery = charging) }
        compose.onNodeWithText("charging").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("discharging").assertDoesNotExist()
        compose.onRoot().captureRoboImage("battery-charging.${theme.key}.png")

        compose.runOnIdle { current.value = machine.copy(battery = BatteryReport.NoBattery) }
        compose.onNodeWithText("no battery").performScrollTo().assertIsDisplayed()
        compose.onAllNodesWithText("— unavailable").assertCountEquals(0)
        compose.onRoot().captureRoboImage("battery-none.${theme.key}.png")

        compose.runOnIdle { current.value = machine.copy(battery = BatteryReport.Present(Battery())) }
        compose.onNodeWithText("no battery").assertDoesNotExist()
        compose.onAllNodesWithText("— unavailable").assertCountEquals(2)
        compose.onAllNodesWithText("— unavailable")[1].performScrollTo().assertIsDisplayed()
        compose.onRoot().captureRoboImage("battery-unavailable.${theme.key}.png")

        compose.runOnIdle { current.value = machine }
        compose.onNodeWithText("no battery").assertDoesNotExist()
        compose.onAllNodesWithText("— unavailable").assertCountEquals(1)
    }

    @Test fun detailShowsWattsTimeLeftAndHealthAtLargeFont() {
        val current = mutableStateOf(machine.copy(battery = discharging))
        compose.setContent { AnshinTheme(theme) {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale = 1.3f)) {
                PulseLayout("machine", onInputChange = {}, onWatch = {}) { PulseView(PulseResult.Ok(current.value), "machine") }
            }
        } }
        compose.onNodeWithText("Machine details").performScrollTo().performClick()
        compose.onNodeWithText("BAT0").performScrollTo().assertIsDisplayed()
        compose.onAllNodesWithText("7.5 W").assertCountEquals(2)
        compose.onNodeWithText("9000 s").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("87.5%").performScrollTo().assertIsDisplayed()
        compose.onRoot().captureRoboImage("battery-detail-discharging-large-font.${theme.key}.png")

        compose.runOnIdle { current.value = machine.copy(battery = charging) }
        compose.onNodeWithText("not discharging").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("9000 s").assertDoesNotExist()

        compose.runOnIdle { current.value = machine.copy(battery = BatteryReport.NoBattery) }
        compose.onNodeWithText("no battery").performScrollTo().assertIsDisplayed()
    }

    @Test fun detailKeepsUnknownTimeLeftUnavailable() {
        val current = mutableStateOf(machine.copy(battery = charging))
        compose.setContent { AnshinTheme(theme) {
            PulseLayout("machine", onInputChange = {}, onWatch = {}) { PulseView(PulseResult.Ok(current.value), "machine") }
        } }
        compose.onNodeWithText("Machine details").performScrollTo().performClick()
        fun report(status: BatteryStatus?) = machine.copy(battery = BatteryReport.Present(Battery(
            pct = 64.0, status = status, powerW = 7.5, batteries = listOf(BatteryRow("BAT0", 64.0, BatteryStatus.Full, 7.5, 87.5)),
        )))
        for (status in listOf(BatteryStatus.Charging, BatteryStatus.NotCharging, BatteryStatus.Full)) {
            compose.runOnIdle { current.value = report(status) }
            compose.onNodeWithText("not discharging").performScrollTo().assertIsDisplayed()
        }
        val unavailableBefore = compose.onAllNodesWithText("— unavailable").fetchSemanticsNodes().size
        for (status in listOf(null, BatteryStatus.Unknown)) {
            compose.runOnIdle { current.value = report(status) }
            compose.onNodeWithText("not discharging").assertDoesNotExist()
            compose.onNodeWithText("Time left").performScrollTo().assertIsDisplayed()
            compose.onAllNodesWithText("— unavailable").assertCountEquals(unavailableBefore + if (status == null) 2 else 1)
        }
    }

    @Test fun desktopFixtureShowsNoBattery() {
        val dir = File(checkNotNull(System.getProperty("anshin.contractFixtures")) { "Gradle sets anshin.contractFixtures" })
        val pulse = checkNotNull(decodePulse(File(dir, "desktop.json").readText()))
        compose.setContent { AnshinTheme(theme) {
            PulseLayout("desktop", onInputChange = {}, onWatch = {}) { PulseView(PulseResult.Ok(pulse), "desktop") }
        } }
        compose.onNodeWithText("no battery").performScrollTo().assertIsDisplayed()
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
