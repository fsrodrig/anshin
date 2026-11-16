package io.github.fsrodrig.anshin

import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.semantics.SemanticsNode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onRoot
import kotlin.time.Duration.Companion.minutes
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

/** What TalkBack is given for each Machine panel: one focus stop, read in one order, and the top line first. */
@RunWith(RobolectricTestRunner::class)
class PanelAnnouncementTest {
    @get:Rule val compose = createComposeRule()

    private fun announced(view: MachineView, machine: Machine = Machines.laptop): SemanticsNode {
        compose.setContent { AnshinTheme(Theme.Default) { MachinePanel(machine, { view }, { Ages.now }, onOpen = {}) } }
        return compose.onNodeWithTag(MACHINE_PANEL_TAG).fetchSemanticsNode()
    }

    private fun assertAnnounces(expected: String, view: MachineView, machine: Machine = Machines.laptop) {
        val panel = announced(view, machine)
        assertEquals(listOf(expected), panel.config.getOrNull(SemanticsProperties.ContentDescription))
        assertNull("the panel's texts are not read again after its announcement", panel.config.getOrNull(SemanticsProperties.Text))
        assertTrue("one focus stop: nothing inside the panel is focusable on its own", panel.children.isEmpty())
        assertNull("a panel is never a live region", panel.config.getOrNull(SemanticsProperties.LiveRegion))
        assertEquals("Open ${machine.name}", panel.config[SemanticsActions.OnClick].label)
    }

    private val laptopValues = "CPU 21 percent. Memory 25 percent. Primary disk 41 percent. CPU temperature 52 degrees Celsius. " +
        "Battery 64 percent, discharging."

    @Test
    fun live() = assertAnnounces("laptop. Live. Verdict ok. $laptopValues", live(Machines.laptopPulse))

    @Test
    fun liveWithAReason() {
        val reason = Reason("filesystem_full", VerdictLevel.Warn, "/ is 91 % full")
        assertAnnounces(
            "laptop. Live. Verdict warn: / is 91 % full. $laptopValues",
            live(Machines.laptopPulse.copy(verdict = Verdict(VerdictLevel.Warn, listOf(reason)))),
        )
    }

    @Test
    fun liveWithEveryValueUnavailableSaysSoAndNeverZero() = assertAnnounces(
        "laptop. Live. Verdict unavailable. CPU unavailable. Memory unavailable. Primary disk unavailable. " +
            "CPU temperature unavailable. Battery unavailable.",
        live(Machines.unavailablePulse),
    )

    @Test
    fun stale() = assertAnnounces(
        "laptop. Stale, last Sample 42 seconds ago, not current. Last Verdict ok, 42 seconds ago. $laptopValues",
        stale(Machines.laptopPulse),
    )

    @Test
    fun unreachableWithLastData() = assertAnnounces(
        "desktop. Unreachable, last Sample 3 minutes ago, not current. Last Verdict ok, 3 minutes ago. " +
            "CPU 8 percent. Memory 38 percent. Primary disk 63 percent. CPU temperature 44 degrees Celsius.",
        unreachable(last = received(Machines.desktopPulse, age = 3.minutes)),
        Machines.desktop,
    )

    @Test
    fun unreachableWithoutData() = assertAnnounces("laptop. Unreachable. No Pulse yet. No current Verdict.", unreachable())

    @Test
    fun connecting() = assertAnnounces("laptop. Connecting. Asking for its first Pulse. No current Verdict.", MachineView.Connecting)

    @Test
    fun forbidden() = assertAnnounces(
        "laptop. Forbidden. Only answers its Owner's untagged devices. No current Verdict.",
        MachineView.Forbidden,
    )

    @Test
    fun incompatible() = assertAnnounces("laptop. Incompatible. Update Anshin and the Agent. No current Verdict.", MachineView.Incompatible)

    @Test
    fun theTopLineIsTheScreensFirstStopAndAHeading() {
        val panels = listOf(panel(Machines.desktop, live(Machines.desktopPulse)), panel(Machines.laptop, MachineView.Forbidden))
        compose.setContent {
            AnshinTheme(Theme.Default) {
                Dashboard(panels, { Ages.now }, onOpen = {}, onAdd = {}, onSettings = {},
                    topLine = { topLine(panels.map { WatchedMachine(it.machine.name, it.view()) }) })
            }
        }
        val stops = focusStops(compose.onRoot().fetchSemanticsNode())
        val first = stops.first()
        assertTrue("the first stop is a heading", first.config.contains(SemanticsProperties.Heading))
        assertEquals(
            listOf("OK · desktop", "No current Verdict from laptop"),
            first.config[SemanticsProperties.Text].map { it.text },
        )
        assertEquals(
            listOf("desktop.", "laptop."),
            stops.mapNotNull { it.config.getOrNull(SemanticsProperties.ContentDescription)?.single()?.substringBefore(' ') },
        )
        assertFalse(stops.any { it.config.getOrNull(SemanticsProperties.Text)?.any { text -> text.text == "anshin" } == true })
    }

    /** The nodes TalkBack stops on, in tree order: whatever has words and isn't hidden from accessibility. */
    private fun focusStops(node: SemanticsNode): List<SemanticsNode> {
        val hidden = node.config.contains(SemanticsProperties.HideFromAccessibility)
        val words = node.config.contains(SemanticsProperties.Text) || node.config.contains(SemanticsProperties.ContentDescription)
        val self = if (!hidden && words) listOf(node) else emptyList()
        return if (hidden) emptyList() else self + node.children.flatMap(::focusStops)
    }
}
