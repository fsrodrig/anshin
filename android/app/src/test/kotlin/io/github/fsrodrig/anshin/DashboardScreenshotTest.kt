package io.github.fsrodrig.anshin

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.getUnclippedBoundsInRoot
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.DpRect
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class DashboardScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    private fun show(panels: List<Panel>, fontScale: Float = 1f) = compose.setContent {
        AnshinTheme(theme) {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale)) {
                Dashboard(
                    panels,
                    now = { Ages.now },
                    onOpen = {},
                    onAdd = {},
                    topLine = { topLine(panels.map { WatchedMachine(it.machine.name, it.view()) }) },
                )
            }
        }
    }

    private fun capture(name: String, panels: List<Panel>, fontScale: Float = 1f) {
        show(panels, fontScale)
        compose.onRoot().captureFrame("dashboard-$name.${theme.key}.png")
    }

    private val bothLive = listOf(
        panel(Machines.desktop, live(Machines.desktopPulse)),
        panel(Machines.laptop, live(Machines.laptopPulse)),
    )

    @Test fun empty() = capture("empty", emptyList())

    @Test fun live() = capture("live", bothLive)

    @Test fun liveAndUnreachable() = capture("live-unreachable", listOf(
        panel(Machines.laptop, live(Machines.laptopPulse)),
        panel(Machines.desktop, unreachable(last = Machines.threeMinutesOld)),
    ))

    @Test fun staleAndConnecting() = capture("stale-connecting", listOf(
        panel(Machines.laptop, stale(Machines.laptopPulse)),
        panel(Machines.desktop, MachineView.Connecting),
    ))

    @Test fun forbiddenAndIncompatible() = capture("forbidden-incompatible", listOf(
        panel(Machines.laptop, MachineView.Forbidden),
        panel(Machines.desktop, MachineView.Incompatible),
    ))

    @Test fun unavailable() = capture("unavailable", listOf(
        panel(Machines.laptop, live(Machines.unavailablePulse)),
        panel(Machines.desktop, unreachable()),
    ))

    @Test fun liveAtLargeFontScale() = capture("live-large-font", bothLive, fontScale = 1.3f)

    @Test fun twoLiveMachinesFitOnOneScreen() {
        show(bothLive)
        val root = compose.onRoot().getUnclippedBoundsInRoot()
        fun inside(bounds: DpRect) = bounds.top >= root.top && bounds.bottom <= root.bottom
        for (name in listOf("desktop", "laptop")) {
            assertTrue("$name's panel is on screen", inside(compose.onNodeWithText(name, useUnmergedTree = true).getUnclippedBoundsInRoot()))
        }
        assertTrue("both panels' last rows are on screen", inside(compose.onNodeWithText("Battery", useUnmergedTree = true).getUnclippedBoundsInRoot()))
        assertTrue("Add machine is on screen", inside(compose.onNodeWithTag(ADD_MACHINE_TAG).getUnclippedBoundsInRoot()))
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
