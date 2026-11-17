package io.github.fsrodrig.anshin

import org.junit.Assert.assertEquals
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasAnyAncestor
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.isHeading
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollToKey
import kotlin.time.Duration.Companion.minutes
import kotlin.time.Duration.Companion.seconds
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** The Machine screen as [AnshinApp] wires it, driven by the views the Watcher emits. */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class MachineScreenTest {
    @get:Rule val compose = createComposeRule()

    private val pulse = DetailSamples.pulse
    private val window = DetailSamples.window
    private val live = live(pulse, window)
    private val last = received(pulse, window)
    private val machine = mutableStateOf(DetailSamples.MACHINE)
    private val current = mutableStateOf<MachineView>(live)
    private val now = mutableStateOf(Ages.now)
    private var edits = 0
    private var backs = 0

    private fun show() {
        compose.setContent {
            AnshinTheme(Theme.Default) {
                MachineScreen(machine.value, machine.value, current.value, { now.value }, Back("Dashboard") { backs++ }, onEdit = { edits++ })
            }
        }
    }

    private fun scrollTo(key: String) = compose.onNodeWithTag(SCREEN_CONTENT_TAG).performScrollToKey(key)

    @Test fun theBarHoldsBackTheMachineNameAndEditAndTheStateLineStaysPinned() {
        show()
        compose.onNode(hasText(DetailSamples.MACHINE) and isHeading()).assertIsDisplayed()
        compose.onNodeWithContentDescription("Back to Dashboard").assertIsDisplayed().performClick()
        compose.onNodeWithContentDescription("Edit ${DetailSamples.MACHINE}").assertIsDisplayed().performClick()
        assertEquals(1, backs)
        assertEquals(1, edits)
        scrollTo("network")
        compose.onNodeWithText("Live").assertIsDisplayed()
        compose.onNodeWithText(" · last Sample 0 s ago").assertIsDisplayed()
        compose.onNode(hasText(DetailSamples.MACHINE) and isHeading()).assertIsDisplayed()
    }

    @Test fun theAddressComesFirstThenTheVerdictAndTheMachinesFactsLast() {
        show()
        val keys = listOf("address", "verdict", "cpu")
        val tops = keys.map { key ->
            compose.onNodeWithTag(key).fetchSemanticsNode().positionInRoot.y
        }
        assertEquals(tops.sorted(), tops)
        compose.onNodeWithText("Overview").assertDoesNotExist()
        scrollTo("machine")
        compose.onNode(hasText("Machine") and isHeading()).assertIsDisplayed()
        scrollTo("agent-version")
        compose.onNodeWithTag("agent-version").assertIsDisplayed()
    }

    @Test fun theScreenStaysOpenAndAgedWhenTheWatcherSaysStale() {
        show()
        compose.onNodeWithText("Live").assertIsDisplayed()
        compose.runOnIdle { current.value = MachineView.Stale(last) }
        compose.onNodeWithText("Stale").assertIsDisplayed()
        compose.onNodeWithText("Live").assertDoesNotExist()
        compose.onNodeWithText(" · last Sample 42 s ago").assertIsDisplayed()
        compose.onNodeWithText("Last Sample 42 s ago. Nothing here is current.").assertIsDisplayed()
        scrollTo("verdict")
        compose.onNodeWithText("Last Verdict").assertIsDisplayed()
        compose.onNodeWithText("ok, 42 s ago").assertIsDisplayed()
        compose.onNodeWithText("ok").assertDoesNotExist()
        scrollTo("cpu-now")
        compose.onNodeWithText("70.2 %").assertIsDisplayed()

        compose.runOnIdle { current.value = live }
        compose.onNodeWithText("Live").assertIsDisplayed()
        compose.onNodeWithText("Stale").assertDoesNotExist()
    }

    @Test fun staleDetailChartSaysLastAndItsAgeAdvancesWithTheWatcherClock() {
        assertChartAgeAdvances(MachineView.Stale(last))
    }

    @Test fun unreachableDetailChartSaysLastAndItsAgeAdvancesWithTheWatcherClock() {
        assertChartAgeAdvances(unreachable(last = last))
    }

    private fun assertChartAgeAdvances(view: MachineView) {
        current.value = view
        show()
        scrollTo("cpu-chart")
        val inChart = hasAnyAncestor(hasTestTag("cpu-chart"))
        fun assertLast(since: String) {
            compose.onNode(inChart and hasContentDescription("last 70 %, $since ago.", substring = true))
                .assertIsDisplayed()
            compose.onAllNodes(inChart and hasContentDescription("now", substring = true)).assertCountEquals(0)
        }
        assertLast("42 s")
        compose.runOnIdle { now.value = Ages.now + 5.seconds }
        assertLast("47 s")
        compose.onAllNodes(inChart and hasContentDescription("42 s ago", substring = true)).assertCountEquals(0)

        compose.runOnIdle { current.value = live }
        compose.onNode(inChart and hasContentDescription("now 70 %.", substring = true)).assertIsDisplayed()
        compose.onAllNodes(inChart and hasContentDescription("last 70 %", substring = true)).assertCountEquals(0)
    }

    @Test fun unreachableWithLastDataSaysWhyWhenItTriesNextAndTheVpnHint() {
        show()
        compose.runOnIdle { current.value = unreachable(last = last, retryIn = 4.seconds, vpnHint = true) }
        compose.onNodeWithText("Unreachable").assertIsDisplayed()
        compose.onNodeWithText("No Pulse from machine. The Machine may be off, its Agent stopped, or this phone off the tailnet.").assertIsDisplayed()
        compose.onNodeWithText("Next try in 4 s").assertIsDisplayed()
        compose.onNodeWithText("VPN looks off").assertIsDisplayed()
        compose.onNodeWithText("Last Sample 42 s ago. Nothing here is current.").assertIsDisplayed()
        compose.runOnIdle { now.value = Ages.now + 5.seconds }
        compose.onNodeWithText("Trying again").assertIsDisplayed()
        compose.onNodeWithText(" · last Sample 47 s ago").assertIsDisplayed()

        compose.runOnIdle { current.value = unreachable(last = last, vpnHint = false) }
        compose.onNodeWithText("VPN looks off").assertDoesNotExist()
        scrollTo("verdict")
        compose.onNodeWithText("ok, 47 s ago").assertIsDisplayed()
    }

    @Test fun anHttpFailureWithLastDataIsUnreachableWithItsCauseSentence() {
        show()
        val causes = mapOf(
            503 to "The Agent on machine can't ask tailscaled who this phone is.",
            421 to "The Agent on machine only answers to its tailnet address or MagicDNS name.",
            500 to "machine answered HTTP 500 instead of a Pulse.",
        )
        for ((status, cause) in causes) {
            compose.runOnIdle { current.value = unreachable(last = last, httpStatus = status) }
            compose.onNodeWithText("Unreachable").assertIsDisplayed()
            compose.onNodeWithText(cause).assertIsDisplayed()
            compose.onNodeWithText("Next try in 4 s").assertIsDisplayed()
            scrollTo("cpu-now")
            compose.onNodeWithText("70.2 %").assertIsDisplayed()
            scrollTo("notice")
        }
    }

    @Test fun aViewWithoutDataSaysWhyInTheSameScreenAndKeepsEdit() {
        show()
        val messages = mapOf(
            MachineView.Forbidden to ("Forbidden" to "This Agent only answers its Owner's untagged devices."),
            MachineView.Incompatible to ("Incompatible" to "The Agent answered in a version of the Pulse this app can't read. Update Anshin and the Agent."),
            MachineView.Connecting to ("Connecting" to "Asking machine for its Pulse."),
            unreachable(httpStatus = 503) to ("Unreachable" to "The Agent on machine can't ask tailscaled who this phone is."),
            unreachable(httpStatus = 421) to ("Unreachable" to "The Agent on machine only answers to its tailnet address or MagicDNS name."),
            unreachable() to ("Unreachable" to "No Pulse from machine. The Machine may be off, its Agent stopped, or this phone off the tailnet."),
        )
        for ((view, words) in messages) {
            val (word, detail) = words
            compose.runOnIdle { current.value = view }
            compose.onNodeWithText(word).assertIsDisplayed()
            compose.onNodeWithText(detail).assertIsDisplayed()
            compose.onNodeWithText(" · last Sample", substring = true).assertDoesNotExist()
            compose.onNodeWithTag("verdict").assertDoesNotExist()
            compose.onNodeWithTag("cpu").assertDoesNotExist()
            compose.onNodeWithContentDescription("Back to Dashboard").assertIsDisplayed()
            compose.onNodeWithContentDescription("Edit machine").assertIsDisplayed()
        }
        compose.onNodeWithText("Next try in 4 s").assertIsDisplayed()
        compose.runOnIdle { current.value = live }
        compose.onNodeWithTag("verdict").assertIsDisplayed()
        compose.onNodeWithText("Live").assertIsDisplayed()
    }

    @Test fun aNewSampleImmediatelyReplacesThePreviousAge() {
        current.value = MachineView.Live(received(pulse, window, age = 2.minutes))
        show()
        compose.onNodeWithText(" · last Sample 2 min ago").assertIsDisplayed()
        compose.runOnIdle { current.value = live }
        compose.onNodeWithText(" · last Sample 0 s ago").assertIsDisplayed()
    }
}
