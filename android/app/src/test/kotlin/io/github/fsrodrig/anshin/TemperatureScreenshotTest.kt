package io.github.fsrodrig.anshin

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.mutableStateOf
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

@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class TemperatureScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    private val sensors = listOf(
        Sensor("k10temp/Tctl", 87.9),
        Sensor("k10temp/Tccd1", 80.5),
        Sensor("amdgpu/edge", 67.0),
        Sensor("it87/temp3", null),
    )
    private val hot = Pulse(schema = 1, hostname = "machine", cpuPct = 21.4, cpuTempC = 87.9, cpuSensor = "k10temp/Tctl", temperatures = sensors)

    @Test fun cpuTemperatureOnTheCard() {
        val current = mutableStateOf(hot)
        compose.setContent { AnshinTheme(theme) {
            PulseLayout("machine", onInputChange = {}, onWatch = {}) { PulseView(PulseResult.Ok(current.value), "machine") }
        } }
        compose.onNodeWithText("88 °C").performScrollTo().assertIsDisplayed()
        compose.onRoot().captureRoboImage("temperature-card.${theme.key}.png")
        compose.runOnIdle { current.value = hot.copy(cpuTempC = null, cpuSensor = null) }
        compose.onNodeWithText("88 °C").assertDoesNotExist()
        compose.onNodeWithText("CPU temperature").performScrollTo().assertIsDisplayed()
        compose.onRoot().captureRoboImage("temperature-card-no-cpu-sensor.${theme.key}.png")
        compose.runOnIdle { current.value = hot.copy(cpuTempC = 0.0) }
        compose.onNodeWithText("0 °C").performScrollTo().assertIsDisplayed()
    }

    @Test fun everySensorOnTheDetailAtLargeFont() {
        compose.setContent { AnshinTheme(theme) {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale = 1.3f)) {
                PulseLayout("machine", onInputChange = {}, onWatch = {}) { PulseView(PulseResult.Ok(hot), "machine") }
            }
        } }
        compose.onNodeWithText("Machine details").performScrollTo().performClick()
        compose.onNodeWithText("CPU sensor").performScrollTo().assertIsDisplayed()
        compose.onAllNodesWithText("k10temp/Tctl")[0].assertIsDisplayed()
        compose.onRoot().captureRoboImage("temperature-detail-cpu-large-font.${theme.key}.png")
        compose.onNodeWithText("amdgpu/edge").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("67.0 °C").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("it87/temp3").performScrollTo().assertIsDisplayed()
        // The row after the last sensor brings that sensor's unavailable temperature into the capture.
        compose.onNodeWithText("Load 1 min").performScrollTo()
        compose.onRoot().captureRoboImage("temperature-detail-sensors-large-font.${theme.key}.png")
        compose.onAllNodesWithText("Back to Machine")[1].performScrollTo().performClick()
        compose.onNodeWithText("CPU temperature").performScrollTo().assertIsDisplayed()
    }

    @Test fun noSensorsAreUnavailableInWords() {
        compose.setContent { AnshinTheme(theme) {
            PulseLayout("machine", onInputChange = {}, onWatch = {}) { MachineDetail(Pulse(schema = 1, hostname = "machine"), onBack = {}) }
        } }
        compose.onNodeWithText("Temperatures").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Sensor").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Load 1 min").performScrollTo()
        compose.onRoot().captureRoboImage("temperature-detail-none.${theme.key}.png")
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
