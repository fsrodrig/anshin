package io.github.fsrodrig.anshin

import android.content.Context
import android.view.accessibility.AccessibilityManager
import androidx.test.core.app.ApplicationProvider
import org.junit.Assert.assertEquals
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotDisplayed
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
import org.robolectric.Shadows.shadowOf
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
        compose.onNodeWithText("last Sample 0\u00A0s\u00A0ago").assertIsDisplayed()
        compose.onNode(hasText(DetailSamples.MACHINE) and isHeading()).assertIsDisplayed()
    }

    @Test fun aSectionHeadingIsPinnedOnlyWhileTouchExplorationIsOff() {
        val accessibility = shadowOf(ApplicationProvider.getApplicationContext<Context>().getSystemService(AccessibilityManager::class.java))
        show()
        val memory = hasText("Memory") and isHeading()
        scrollTo("swap-total")
        compose.onNode(memory).assertIsDisplayed()

        // TalkBack focus would land under a pinned heading, so with touch exploration the heading scrolls away.
        compose.runOnIdle { accessibility.setTouchExplorationEnabled(true) }
        scrollTo("swap-total")
        compose.onNode(memory).assertIsNotDisplayed()
        scrollTo("memory")
        compose.onNode(memory).assertIsDisplayed()

        compose.runOnIdle { accessibility.setTouchExplorationEnabled(false) }
        scrollTo("swap-total")
        compose.onNode(memory).assertIsDisplayed()
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
        compose.onNodeWithText("last Sample 42\u00A0s\u00A0ago").assertIsDisplayed()
        compose.onNodeWithText(NOT_CURRENT).assertIsDisplayed()
        scrollTo("verdict")
        compose.onNodeWithText("Last Verdict").assertIsDisplayed()
        compose.onNodeWithText("ok, 42\u00A0s\u00A0ago").assertIsDisplayed()
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
        scrollTo("cpu-now")
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
        compose.onNodeWithText("Next try in 4\u00A0s").assertIsDisplayed()
        compose.onNodeWithText("VPN looks off").assertIsDisplayed()
        compose.onNodeWithText(NOT_CURRENT).assertIsDisplayed()
        compose.runOnIdle { now.value = Ages.now + 5.seconds }
        compose.onNodeWithText("Trying again").assertIsDisplayed()
        compose.onNodeWithText("last Sample 47\u00A0s\u00A0ago").assertIsDisplayed()

        compose.runOnIdle { current.value = unreachable(last = last, vpnHint = false) }
        compose.onNodeWithText("VPN looks off").assertDoesNotExist()
        scrollTo("verdict")
        compose.onNodeWithText("ok, 47\u00A0s\u00A0ago").assertIsDisplayed()
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
            compose.onNodeWithText("Next try in 4\u00A0s").assertIsDisplayed()
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
        compose.onNodeWithText("Next try in 4\u00A0s").assertIsDisplayed()
        compose.runOnIdle { current.value = live }
        compose.onNodeWithTag("verdict").assertIsDisplayed()
        compose.onNodeWithText("Live").assertIsDisplayed()
    }

    @Test fun aSectionHeadingStaysOnScreenWhileItsSectionScrollsUnderIt() {
        show()
        scrollTo("disk-write-chart")
        compose.onNode(hasText("Storage") and isHeading()).assertIsDisplayed()
        scrollTo("sent-chart")
        compose.onNode(hasText("Network") and isHeading()).assertIsDisplayed()
        compose.onNode(hasText("Storage") and isHeading()).assertDoesNotExist()
    }

    @Test fun aSeriesWithNoMeasuredValueDrawsOnlyItsWordsAndKeepsItsSummary() {
        val sparse = window.copy(window = window.window.copy(swapUsedBytes = null, cpuPct = null))
        current.value = live(pulse, sparse)
        show()
        // The CPU chart is drawn in one item with its display number.
        for ((stop, key, label) in listOf(Triple("swap-chart", "swap-chart", "Swap used"), Triple("cpu-now", "cpu-chart", "CPU"))) {
            scrollTo(stop)
            val inChart = hasTestTag(key) or hasAnyAncestor(hasTestTag(key))
            // A chart under its display number shows its name once there is no plot to sit under the number.
            compose.onNode(inChart and hasText(label)).assertIsDisplayed()
            compose.onNode(inChart and hasText("— unavailable, last 30 min") and hasContentDescription("$label, last 30 min: unavailable."))
                .assertIsDisplayed()
            compose.onAllNodes(inChart and hasContentDescription("", substring = true)).assertCountEquals(1)
        }
    }

    @Test fun aNewSampleImmediatelyReplacesThePreviousAge() {
        current.value = MachineView.Live(received(pulse, window, age = 2.minutes))
        show()
        compose.onNodeWithText("last Sample 2\u00A0min\u00A0ago").assertIsDisplayed()
        compose.runOnIdle { current.value = live }
        compose.onNodeWithText("last Sample 0\u00A0s\u00A0ago").assertIsDisplayed()
    }
}
