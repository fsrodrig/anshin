package io.github.fsrodrig.anshin

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.getUnclippedBoundsInRoot
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.DpRect
import androidx.compose.ui.geometry.Rect
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsNode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performSemanticsAction
import androidx.compose.ui.text.TextLayoutResult
import androidx.compose.ui.unit.toSize
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
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
                    onSettings = {},
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

    @Test fun warn() = capture("warn", warnAndOk)

    // Tall enough to hold every row of both panels, so the golden shows what scrolling reaches.
    @Test @Config(qualifiers = "w360dp-h1600dp-xhdpi")
    fun liveAtDoubleFontOnASmallPhone() = capture("live-double-font", warnAndOk, fontScale = 2f)

    @Test @Config(qualifiers = "w360dp-h1600dp-xhdpi")
    fun historyAtDoubleFontOnASmallPhone() = capture("history-double-font", listOf(
        panel(Machines.laptop, stale(Machines.laptopPulse.copy(verdict = Machines.diskWarn))),
        panel(Machines.desktop, unreachable(last = Machines.threeMinutesOld)),
    ), fontScale = 2f)

    @Test @Config(qualifiers = "w360dp-h1600dp-xhdpi")
    fun unavailableAtDoubleFontOnASmallPhone() = capture("unavailable-double-font", listOf(
        panel(Machines.laptop, live(Machines.unavailablePulse)),
        panel(Machines.desktop, MachineView.Forbidden),
    ), fontScale = 2f)

    @Test fun twoLiveMachinesFitOnOneScreen() {
        show(bothLive)
        val root = compose.onRoot().getUnclippedBoundsInRoot()
        fun inside(bounds: DpRect) = bounds.top >= root.top && bounds.bottom <= root.bottom
        val panels = compose.onAllNodesWithTag(MACHINE_PANEL_TAG).fetchSemanticsNodes()
        assertEquals(2, panels.size)
        for (panel in panels) {
            assertTrue("every row of ${panel.config[SemanticsProperties.ContentDescription]} is on screen", inside(panel.unclippedBoundsInRoot()))
        }
        assertTrue("Add machine is on screen", inside(compose.onNodeWithTag(ADD_MACHINE_TAG).getUnclippedBoundsInRoot()))
    }

    @Test fun liveTextsReadAtDoubleFontOnASmallPhone() = assertPanelsRead(warnAndOk, fontScale = 2f)

    @Test fun historyTextsReadAtDoubleFontOnASmallPhone() = assertPanelsRead(listOf(
        panel(Machines.laptop, stale(Machines.laptopPulse.copy(verdict = Machines.diskWarn))),
        panel(Machines.desktop, unreachable(last = Machines.threeMinutesOld)),
    ), fontScale = 2f)

    @Test fun unavailableTextsReadAtDoubleFontOnASmallPhone() = assertPanelsRead(listOf(
        panel(Machines.laptop, live(Machines.unavailablePulse)),
        panel(Machines.desktop, unreachable(httpStatus = 500)),
    ), fontScale = 2f)

    @Test fun everyPanelTextReadsAtNormalFont() = assertPanelsRead(warnAndOk, fontScale = 1f)

    private fun assertPanelsRead(panels: List<Panel>, fontScale: Float) {
        show(panels, fontScale)
        val texts = compose.onAllNodes(hasAnyAncestor(hasTestTag(MACHINE_PANEL_TAG)) and SemanticsMatcher.keyIsDefined(SemanticsActions.GetTextLayoutResult), useUnmergedTree = true)
        val nodes = texts.fetchSemanticsNodes()
        assertTrue("found the panels' texts", nodes.size >= 2 * 6)
        val viewport = compose.onRoot().getUnclippedBoundsInRoot()
        nodes.indices.forEach { i ->
            val node = texts[i]
            val semantics = node.fetchSemanticsNode()
            val layouts = mutableListOf<TextLayoutResult>()
            node.performSemanticsAction(SemanticsActions.GetTextLayoutResult) { it(layouts) }
            val layout = layouts.single()
            val text = layout.layoutInput.text.text
            // A text's own box is what is drawn, so each line has to end inside it; the layout's own overflow flag also trips
            // when a FlowRow measures it wider than it is placed.
            val drawn = semantics.size
            assertFalse("\"$text\" overflows its height", layout.didOverflowHeight)
            assertTrue("\"$text\" is clipped at the bottom", layout.getLineBottom(layout.lineCount - 1) <= drawn.height + 0.5f)
            for (line in 0 until layout.lineCount) {
                assertFalse("\"$text\" is ellipsized", layout.isLineEllipsized(line))
                assertTrue("\"$text\" is clipped at the end of line $line", layout.getLineRight(line) <= drawn.width + 0.5f)
                val end = layout.getLineEnd(line)
                if (line < layout.lineCount - 1) {
                    assertFalse("\"$text\" breaks inside a word", text[end - 1].isLetterOrDigit() && text[end].isLetterOrDigit())
                }
            }
            val panel = generateSequence(semantics) { it.parent }.first { it.config.getOrNull(SemanticsProperties.TestTag) == MACHINE_PANEL_TAG }
            val bounds = semantics.unclippedBoundsInRoot()
            val box = panel.unclippedBoundsInRoot()
            assertTrue("\"$text\" is inside its panel", bounds.left >= box.left && bounds.right <= box.right && bounds.top >= box.top && bounds.bottom <= box.bottom)
            nodes.filter { it.id != semantics.id }.forEach { other ->
                val o = other.unclippedBoundsInRoot()
                val overlaps = bounds.left < o.right && o.left < bounds.right && bounds.top < o.bottom && o.top < bounds.bottom
                assertFalse("\"$text\" overlaps another text", overlaps)
            }
            node.performScrollTo()
            val shown = node.getUnclippedBoundsInRoot()
            assertTrue("scrolling brings \"$text\" fully on screen", shown.top >= viewport.top && shown.bottom <= viewport.bottom)
        }
    }

    private fun SemanticsNode.unclippedBoundsInRoot(): DpRect = with(compose.density) {
        val r = Rect(positionInRoot, size.toSize())
        DpRect(r.left.toDp(), r.top.toDp(), r.right.toDp(), r.bottom.toDp())
    }

    private val warnAndOk = listOf(
        panel(Machines.laptop, live(Machines.laptopPulse)),
        panel(Machines.desktop, live(Machines.desktopPulse.copy(
            filesystems = listOf(Filesystem(majMin = "0:36", mountPoint = "/", usedPct = 91.0)),
            verdict = Machines.diskWarn,
        ))),
    )

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
