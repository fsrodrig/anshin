package io.github.fsrodrig.anshin

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performScrollToKey
import androidx.compose.ui.unit.Density
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** The battery, filesystem, sensor and interface edge cases on the detail, in the default theme. */
@RunWith(RobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class DetailEdgeCasesScreenshotTest {
    @get:Rule val compose = createComposeRule()

    private val machine = Pulse(schema = PULSE_SCHEMA, kind = PulseKind.Full, hostname = DetailSamples.MACHINE)
    private val current = mutableStateOf(machine)

    private fun show(pulse: Pulse, fontScale: Float = 1f) {
        current.value = pulse
        compose.setContent {
            AnshinTheme(Theme.Default) {
                val density = LocalDensity.current
                CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale)) {
                    MachineScreen(DetailSamples.state(pulse = current.value, window = null))
                }
            }
        }
    }

    // A pinned section heading covers the item a scroll brings to the top, so each shot scrolls to the item above it.
    private fun scrollTo(key: String) {
        compose.onNodeWithTag(SCREEN_CONTENT_TAG).performScrollToKey(key)
    }

    private fun inRow(key: String, text: String, substring: Boolean = false): SemanticsMatcher =
        hasText(text, substring = substring) and (hasTestTag(key) or hasAnyAncestor(hasTestTag(key)))

    @Test fun batteryDischargingAtLargeFont() {
        show(machine.copy(battery = DetailSamples.battery), fontScale = 1.3f)
        scrollTo("battery")
        compose.onAllNodesWithText("7.5 W", substring = true).assertCountEquals(2)
        compose.onNodeWithText("2 h 30 min").assertIsDisplayed()
        // Each pair of a meta line is its own text, so a wrap never leaves a separator dangling.
        compose.onNodeWithText("health 87.5 %").assertIsDisplayed()
        compose.onNodeWithText("BAT0").assertIsDisplayed()
        compose.onRoot().captureFrame("detail-battery-discharging-large-font.${Theme.Default.key}.png")
    }

    @Test fun batteryTimeLeftByStatus() {
        fun report(status: BatteryStatus?) = machine.copy(battery = BatteryReport.Present(Battery(
            pct = 64.0, status = status, powerW = 7.5, timeLeftS = 9000.0,
            batteries = listOf(BatteryRow("BAT0", 64.0, BatteryStatus.Full, 7.5, 87.5)),
        )))
        show(report(BatteryStatus.Charging))
        for (status in listOf(BatteryStatus.Charging, BatteryStatus.NotCharging, BatteryStatus.Full)) {
            compose.runOnIdle { current.value = report(status) }
            scrollTo("battery-time-left")
            compose.onNode(inRow("battery-time-left", "not discharging")).assertIsDisplayed()
        }
        for (status in listOf(null, BatteryStatus.Unknown)) {
            compose.runOnIdle { current.value = report(status) }
            scrollTo("battery-time-left")
            compose.onNode(inRow("battery-time-left", UNAVAILABLE)).assertIsDisplayed()
            compose.onNodeWithText("not discharging").assertDoesNotExist()
        }
        compose.runOnIdle { current.value = machine.copy(battery = BatteryReport.NoBattery) }
        scrollTo("no-battery")
        compose.onNodeWithText("no battery").assertIsDisplayed()
        for (report in listOf(BatteryReport.Present(Battery()), BatteryReport.NotReported)) {
            compose.runOnIdle { current.value = machine.copy(battery = report) }
            scrollTo("battery-unavailable")
            compose.onNode(inRow("battery-unavailable", UNAVAILABLE)).assertIsDisplayed()
            compose.onNodeWithTag("battery-time-left").assertDoesNotExist()
            compose.onNodeWithText("no battery").assertDoesNotExist()
        }
    }

    @Test fun filesystemsAtLargeFont() {
        val longPath = "/mnt/archive/very-long-filesystem-mount-point/with-more-components"
        show(machine.copy(primaryDisk = "0:35", filesystems = listOf(
            Filesystem("8:1", "/boot", "ext4", 1024, 512, 512, 50.0, 0.0),
            Filesystem("0:35", "/", "btrfs", 4096, 0, 4096, 0.0, 0.0),
            Filesystem("8:2", longPath, "xfs", ageS = 90.0),
        )), fontScale = 1.3f)
        scrollTo("storage")
        compose.onNode(inRow("fs:1", "0.0 %")).assertIsDisplayed()
        compose.onNode(inRow("fs:1", "primary", substring = true)).assertIsDisplayed()
        compose.onRoot().captureFrame("detail-filesystems-large-font.${Theme.Default.key}.png")
        scrollTo("fs:1")
        compose.onNodeWithText(longPath).assertIsDisplayed()
        compose.onNode(inRow("fs:2", UNAVAILABLE)).assertIsDisplayed()
        compose.onNode(inRow("fs:2", "Capacity age 90 s")).assertIsDisplayed()
        compose.onRoot().captureFrame("detail-filesystems-long-path-large-font.${Theme.Default.key}.png")
    }

    @Test fun sensorsAtLargeFont() {
        show(machine.copy(cpuTempC = 87.9, cpuSensor = "k10temp/Tctl", temperatures = listOf(
            Sensor("k10temp/Tctl", 87.9),
            Sensor("k10temp/Tccd1", 80.5),
            Sensor("amdgpu/edge", 67.0, ageS = 4),
            Sensor("it87/temp3", null),
        )), fontScale = 1.3f)
        scrollTo("temperatures")
        compose.onAllNodesWithText("CPU sensor").assertCountEquals(1)
        compose.onNode(inRow("sensor:0", "CPU sensor")).assertIsDisplayed()
        compose.onNodeWithText("67.0 °C (4 s ago)").assertIsDisplayed()
        compose.onNode(inRow("sensor:3", UNAVAILABLE)).assertIsDisplayed()
        compose.onRoot().captureFrame("detail-temperatures-large-font.${Theme.Default.key}.png")
    }

    @Test fun networkInterfaces() {
        show(machine.copy(netRxBps = 75_304, netTxBps = 23_269, netInterfaces = listOf(
            NetInterface("eth0", true, 75_262, 23_269),
            NetInterface("lo", false, 0, 0),
            NetInterface("wg0", null, null, 5),
        )))
        scrollTo("network-note")
        compose.onNode(inRow("if:0", "device")).assertIsDisplayed()
        compose.onNode(inRow("if:1", "no device")).assertIsDisplayed()
        compose.onNode(inRow("if:2", "device $UNAVAILABLE")).assertIsDisplayed()
        compose.onNode(inRow("if:2", "received $UNAVAILABLE", substring = true)).assertIsDisplayed()
        compose.onRoot().captureFrame("detail-network-interfaces.${Theme.Default.key}.png")
    }
}
