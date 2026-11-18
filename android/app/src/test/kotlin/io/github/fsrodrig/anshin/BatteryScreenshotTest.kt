package io.github.fsrodrig.anshin

import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performScrollToKey
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

    private val machine = Pulse(
        schema = 1, hostname = "machine", cpuPct = 21.0, cpuTempC = 52.1, cpuSensor = "k10temp/Tctl", memUsedPct = 40.0, swapTotalBytes = 2048, swapUsedBytes = 0,
        filesystems = listOf(Filesystem("0:35", "/", "btrfs", 4096, 1024, 3072, 25.0, 0.0)), primaryDisk = "0:35",
    )
    private val discharging = BatteryReport.Present(Battery(64.0, BatteryStatus.Discharging, 7.5, 9000.0,
        listOf(BatteryRow("BAT0", 64.0, BatteryStatus.Discharging, 7.5, 87.5))))
    private val charging = BatteryReport.Present(Battery(64.0, BatteryStatus.Charging, 15.0, null,
        listOf(BatteryRow("BAT0", 64.0, BatteryStatus.Charging, 15.0, 87.5))))

    // A pinned section heading covers the item a scroll brings to the top, so each shot scrolls to the item above it.
    private fun scrollTo(key: String) = compose.onNodeWithTag(SCREEN_CONTENT_TAG).performScrollToKey(key)

    @Test fun theMachineScreenShowsChargeAndState() {
        val current = mutableStateOf(machine.copy(battery = discharging))
        compose.setContent { AnshinTheme(theme) { MachineScreen(live(current.value)) } }
        scrollTo("battery")
        compose.onNode(inItem("battery-now", "64.0 % discharging")).assertIsDisplayed()
        compose.onRoot().captureFrame("battery-discharging.${theme.key}.png")

        compose.runOnIdle { current.value = machine.copy(battery = charging) }
        scrollTo("battery")
        compose.onNode(inItem("battery-now", "64.0 % charging")).assertIsDisplayed()
        compose.onAllNodes(inItem("battery-now", "discharging", substring = true)).assertCountEquals(0)
        compose.onRoot().captureFrame("battery-charging.${theme.key}.png")

        compose.runOnIdle { current.value = machine.copy(battery = BatteryReport.NoBattery) }
        scrollTo("battery")
        compose.onNode(inItem("no-battery", "no battery")).assertIsDisplayed()
        compose.onRoot().captureFrame("battery-none.${theme.key}.png")

        compose.runOnIdle { current.value = machine.copy(battery = BatteryReport.Present(Battery())) }
        scrollTo("battery")
        compose.onNode(inItem("battery-unavailable", UNAVAILABLE)).assertIsDisplayed()
        compose.onAllNodes(hasText("no battery")).assertCountEquals(0)
        compose.onRoot().captureFrame("battery-unavailable.${theme.key}.png")

        compose.runOnIdle { current.value = machine }
        scrollTo("battery")
        compose.onNode(inItem("battery-unavailable", UNAVAILABLE)).assertIsDisplayed()
        compose.onAllNodes(hasText("no battery")).assertCountEquals(0)
    }

    @Test fun desktopFixtureShowsNoBattery() {
        val dir = File(checkNotNull(System.getProperty("anshin.contractFixtures")) { "Gradle sets anshin.contractFixtures" })
        val pulse = checkNotNull(decodePulse(File(dir, "desktop.json").readText()))
        compose.setContent { AnshinTheme(theme) { MachineScreen(live(pulse), "desktop") } }
        scrollTo("battery")
        compose.onNode(inItem("no-battery", "no battery")).assertIsDisplayed()
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
