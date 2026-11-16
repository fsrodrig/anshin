package io.github.fsrodrig.anshin

import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.assertCountEquals
import kotlin.time.Duration.Companion.seconds
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

private const val MACHINE = "machine"

/** Stale and Unreachable data is never presented as current, and the VPN hint is only words. */
@RunWith(RobolectricTestRunner::class)
class MachineViewScreenTest {
    @get:Rule val compose = createComposeRule()

    private val crit = Pulse(schema = PULSE_SCHEMA, kind = PulseKind.Full, hostname = MACHINE, cpuPct = 21.0,
        verdict = Verdict(VerdictLevel.Crit, listOf(Reason("cpu_hot", VerdictLevel.Crit, "CPU at 97.0 °C"))))

    private fun show(view: MachineView, now: () -> kotlin.time.ComparableTimeMark = { Ages.now }) = compose.setContent {
        AnshinTheme(Theme.Default) {
            MachineScreen(topLine(listOf(WatchedMachine(MACHINE, view)))) {
                PulseView(view, MACHINE, now = now)
            }
        }
    }

    @Test
    fun staleDataShowsItsAgeAndItsVerdictOnlyAsHistory() {
        val now = mutableStateOf(Ages.now)
        show(stale(crit, WindowSamples.endingAt(cpuNow = 21.0, memoryNow = 25.0))) { now.value }
        compose.onNodeWithText("Stale").assertExists()
        compose.onNodeWithText("Live").assertDoesNotExist()
        compose.onNodeWithText("Last Sample 42 s ago. Nothing here is current.").assertExists()
        compose.onNodeWithText("Last Verdict").assertExists()
        compose.onNodeWithText("crit, 42 s ago").assertExists()
        compose.onNodeWithText("crit").assertDoesNotExist()
        compose.onNodeWithText("CRIT · $MACHINE").assertDoesNotExist()
        compose.onNodeWithText("No current Verdict from $MACHINE").assertExists()
        compose.onNodeWithText("Machine details").assertDoesNotExist()
        compose.onAllNodes(SemanticsMatcher("a chart that says now") { node ->
            node.config.getOrElse(SemanticsProperties.ContentDescription) { emptyList() }.any { "now" in it }
        }).assertCountEquals(0)
        val charts = compose.onAllNodes(SemanticsMatcher("a historical chart") { node ->
            node.config.getOrElse(SemanticsProperties.ContentDescription) { emptyList() }.any { "last 21 %, 42 s ago" in it }
        })
        assertTrue(charts.fetchSemanticsNodes().isNotEmpty())

        compose.runOnIdle { now.value = Ages.now + 1.seconds }
        compose.onNodeWithText("Last Sample 43 s ago. Nothing here is current.").assertExists()
    }

    @Test
    fun unreachableKeepsItsLastDataAsHistoryAndSaysWhenItTriesNext() {
        val now = mutableStateOf(Ages.now)
        show(unreachable(last = received(crit), retryIn = 4.seconds)) { now.value }
        compose.onNodeWithText("Unreachable").assertExists()
        compose.onNodeWithText("Next try in 4 s").assertExists()
        compose.onNodeWithText("Last Sample 42 s ago. Nothing here is current.").assertExists()
        compose.onNodeWithText("Live").assertDoesNotExist()
        compose.onNodeWithText("No current Verdict from $MACHINE").assertExists()
        compose.runOnIdle { now.value = Ages.now + 5.seconds }
        compose.onNodeWithText("Trying again").assertExists()
    }

    @Test
    fun theVpnHintAppearsOnlyWhenTheWatcherSaysSo() {
        val view = mutableStateOf(unreachable(vpnHint = false))
        compose.setContent { AnshinTheme(Theme.Default) { PulseView(view.value, MACHINE) } }
        compose.onAllNodesWithText("VPN looks off").assertCountEquals(0)
        compose.runOnIdle { view.value = unreachable(vpnHint = true) }
        compose.onNodeWithText("VPN looks off").assertExists()
    }
}
