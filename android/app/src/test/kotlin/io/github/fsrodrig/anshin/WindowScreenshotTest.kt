package io.github.fsrodrig.anshin

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.ui.Modifier
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

/** The CPU and memory charts of the Live screen, for each shape a Window can take. */
@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class WindowScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    private fun capture(name: String, held: HeldWindow?) {
        compose.setContent {
            AnshinTheme(theme) {
                val colors = AnshinTheme.colors
                Column(
                    Modifier.fillMaxSize().background(colors.background).padding(24.dp),
                    verticalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    Text("CPU", color = colors.muted, fontSize = 13.sp)
                    WindowChart("CPU", held, { it.cpuPct }, colors.cpu)
                    Text("Memory", color = colors.muted, fontSize = 13.sp)
                    WindowChart("Memory", held, { it.memUsedPct }, colors.memory)
                }
            }
        }
        compose.onRoot().captureRoboImage("window-$name.${theme.key}.png")
    }

    @Test fun full() = capture("full", WindowSamples.samples(900))

    @Test fun gap() = capture("gap", WindowSamples.withGap())

    @Test fun longGap() = capture("long-gap", WindowSamples.withGap(gapMs = 720_000, endsBeforeNewestMs = 300_000))

    @Test fun short() = capture("short", WindowSamples.samples(60))

    @Test fun cpuUnavailable() = capture("cpu-unavailable", WindowSamples.samples(900, cpu = { null }))

    @Test fun none() = capture("none", null)

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
