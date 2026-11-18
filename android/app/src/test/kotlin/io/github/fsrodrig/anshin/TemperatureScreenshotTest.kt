package io.github.fsrodrig.anshin

import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performScrollToKey
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class TemperatureScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    private val sensors = listOf(
        Sensor("k10temp/Tctl", 87.9),
        Sensor("k10temp/Tccd1", 80.5),
        Sensor("amdgpu/edge", 67.0, ageS = 4),
        Sensor("it87/temp3", null),
    )
    private val hot = Pulse(schema = 1, hostname = "machine", cpuPct = 21.4, cpuTempC = 87.9, cpuSensor = "k10temp/Tctl", temperatures = sensors)

    // A pinned section heading covers the item a scroll brings to the top, so each shot scrolls to the item above it.
    private fun scrollTo(key: String) = compose.onNodeWithTag(SCREEN_CONTENT_TAG).performScrollToKey(key)

    @Test fun cpuTemperatureOnTheMachineScreen() {
        val current = mutableStateOf(hot)
        compose.setContent { AnshinTheme(theme) { MachineScreen(live(current.value)) } }
        scrollTo("load-chart")
        compose.onNode(inItem("cpu-temperature", "87.9 °C")).assertIsDisplayed()
        compose.onRoot().captureFrame("temperature-card.${theme.key}.png")
        compose.runOnIdle { current.value = hot.copy(cpuTempC = null, cpuSensor = null) }
        scrollTo("load-chart")
        compose.onNode(inItem("cpu-temperature", UNAVAILABLE)).assertIsDisplayed()
        compose.onAllNodes(inItem("cpu-temperature", "87.9 °C")).assertCountEquals(0)
        compose.onRoot().captureFrame("temperature-card-no-cpu-sensor.${theme.key}.png")
        compose.runOnIdle { current.value = hot.copy(cpuTempC = 0.0) }
        scrollTo("load-chart")
        compose.onNode(inItem("cpu-temperature", "0.0 °C")).assertIsDisplayed()
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
