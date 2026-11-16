package io.github.fsrodrig.anshin

import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performScrollTo
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
        schema = 1, hostname = "machine", cpuPct = 21.0, cpuTempC = 52.1, cpuSensor = "k10temp/Tctl", memUsedPct = 40.0, swapTotalBytes = 2048, swapUsedBytes = 0,
        filesystems = listOf(Filesystem("0:35", "/", "btrfs", 4096, 1024, 3072, 25.0, 0.0)), primaryDisk = "0:35",
    )
    private val discharging = BatteryReport.Present(Battery(64.0, BatteryStatus.Discharging, 7.5, 9000.0,
        listOf(BatteryRow("BAT0", 64.0, BatteryStatus.Discharging, 7.5, 87.5))))
    private val charging = BatteryReport.Present(Battery(64.0, BatteryStatus.Charging, 15.0, null,
        listOf(BatteryRow("BAT0", 64.0, BatteryStatus.Charging, 15.0, 87.5))))

    @Test fun cardShowsChargeAndState() {
        val current = mutableStateOf(machine.copy(battery = discharging))
        compose.setContent { AnshinTheme(theme) {
            MachineScreen() { PulseView(live(current.value), "machine") }
        } }
        compose.onNodeWithText("64%", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("discharging", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        compose.onRoot().captureFrame("battery-discharging.${theme.key}.png")

        compose.runOnIdle { current.value = machine.copy(battery = charging) }
        compose.onNodeWithText("charging", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("discharging", useUnmergedTree = true).assertDoesNotExist()
        compose.onRoot().captureFrame("battery-charging.${theme.key}.png")

        compose.runOnIdle { current.value = machine.copy(battery = BatteryReport.NoBattery) }
        compose.onNodeWithText("no battery", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        compose.onAllNodesWithText("— unavailable", useUnmergedTree = true).assertCountEquals(0)
        compose.onRoot().captureFrame("battery-none.${theme.key}.png")

        compose.runOnIdle { current.value = machine.copy(battery = BatteryReport.Present(Battery())) }
        compose.onNodeWithText("no battery", useUnmergedTree = true).assertDoesNotExist()
        compose.onAllNodesWithText("— unavailable", useUnmergedTree = true).assertCountEquals(2)
        compose.onAllNodesWithText("— unavailable", useUnmergedTree = true)[1].performScrollTo().assertIsDisplayed()
        compose.onRoot().captureFrame("battery-unavailable.${theme.key}.png")

        compose.runOnIdle { current.value = machine }
        compose.onNodeWithText("no battery", useUnmergedTree = true).assertDoesNotExist()
        compose.onAllNodesWithText("— unavailable", useUnmergedTree = true).assertCountEquals(1)
    }

    @Test fun desktopFixtureShowsNoBattery() {
        val dir = File(checkNotNull(System.getProperty("anshin.contractFixtures")) { "Gradle sets anshin.contractFixtures" })
        val pulse = checkNotNull(decodePulse(File(dir, "desktop.json").readText()))
        compose.setContent { AnshinTheme(theme) {
            MachineScreen(name = "desktop") { PulseView(live(pulse), "desktop") }
        } }
        compose.onNodeWithText("no battery", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
