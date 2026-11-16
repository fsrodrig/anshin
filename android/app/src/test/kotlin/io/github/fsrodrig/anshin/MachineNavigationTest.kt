package io.github.fsrodrig.anshin

import org.junit.Assert.assertEquals
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToKey
import androidx.test.espresso.Espresso
import kotlin.time.Duration.Companion.minutes
import kotlin.time.Duration.Companion.seconds
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** The card and the detail as [PulseScreen] wires them, driven by the views the Watcher emits. */
@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class MachineNavigationTest {
    @get:Rule val compose = createComposeRule()

    private val pulse = DetailSamples.pulse
    private val window = DetailSamples.window
    private val live = live(pulse, window)
    private val last = received(pulse, window)
    private val machine = mutableStateOf(DetailSamples.MACHINE)
    private val current = mutableStateOf<MachineView>(live)
    private val now = mutableStateOf(Ages.now)

    private fun show() {
        compose.setContent {
            AnshinTheme(Theme.Default) {
                val view = current.value
                MachineScreens(machine.value, view, now = { now.value }) { open ->
                    PulseLayout(machine.value, onInputChange = {}, onWatch = {}) {
                        PulseView(view, machine.value, now = { now.value }, onDetail = open)
                    }
                }
            }
        }
    }

    private fun openDetail() {
        compose.onNodeWithText("Machine details").performScrollTo().performClick()
        compose.onNodeWithText("Back to Machine").assertIsDisplayed()
    }

    private fun assertOnCard() {
        compose.onNodeWithText("Watch").assertIsDisplayed()
        compose.onNodeWithText("Back to Machine").assertDoesNotExist()
    }

    private fun scrollTo(key: String) = compose.onNodeWithTag("machine-detail").performScrollToKey(key)

    @Test fun machineDetailsOpensTheDetailAndBothBacksReturnToTheCard() {
        show()
        openDetail()
        compose.onNodeWithText("Watch").assertDoesNotExist()
        compose.onNodeWithText("Machine").assertDoesNotExist()
        Espresso.pressBack()
        assertOnCard()
        openDetail()
        compose.onNodeWithText("Back to Machine").performClick()
        assertOnCard()
    }

    @Test fun theDetailStaysOpenAndAgedWhenTheWatcherSaysStale() {
        show()
        openDetail()
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

    @Test fun unreachableWithLastDataSaysWhyWhenItTriesNextAndTheVpnHint() {
        show()
        openDetail()
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

    @Test fun anHttpFailureWithLastDataIsUnreachableUnderTheCardsCauseSentence() {
        show()
        openDetail()
        val causes = mapOf(
            503 to "The Agent on machine can't ask tailscaled who this phone is.",
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

    @Test fun aViewWithoutDataShowsTheCardsMessageInsideTheDetail() {
        show()
        openDetail()
        val messages = mapOf(
            MachineView.Forbidden to "This Agent only answers its Owner's untagged devices.",
            MachineView.Incompatible to "The Agent answered in a version of the Pulse this app can't read. Update Anshin and the Agent.",
            MachineView.Connecting to "Asking machine for its Pulse.",
            unreachable(httpStatus = 503) to "The Agent on machine can't ask tailscaled who this phone is.",
            unreachable() to "No Pulse from machine. The Machine may be off, its Agent stopped, or this phone off the tailnet.",
        )
        for ((view, detail) in messages) {
            compose.runOnIdle { current.value = view }
            compose.onNodeWithText(detail).assertIsDisplayed()
            compose.onNodeWithText("Back to Machine").assertIsDisplayed()
            compose.onNodeWithTag("machine-detail").assertDoesNotExist()
            compose.onNodeWithText("Watch").assertDoesNotExist()
        }
        compose.onNodeWithText("Next try in 4 s").assertIsDisplayed()
        compose.runOnIdle { current.value = live }
        compose.onNodeWithTag("machine-detail").assertIsDisplayed()
        compose.onNodeWithText("Live").assertIsDisplayed()
    }

    @Test fun aNewSampleImmediatelyReplacesThePreviousAge() {
        current.value = MachineView.Live(received(pulse, window, age = 2.minutes))
        show()
        openDetail()
        compose.onNodeWithText(" · last Sample 2 min ago").assertIsDisplayed()
        compose.runOnIdle { current.value = live }
        compose.onNodeWithText(" · last Sample 0 s ago").assertIsDisplayed()
    }

    @Test fun tappingACardWithDataOpensTheDetail() {
        show()
        for (view in listOf(live, MachineView.Stale(last), unreachable(last = last))) {
            compose.runOnIdle { current.value = view }
            val card = compose.onNodeWithTag("machine-card")
            assertEquals("Machine details", card.fetchSemanticsNode().config[SemanticsActions.OnClick].label)
            card.performClick()
            compose.onNodeWithText("Back to Machine").assertIsDisplayed()
            compose.onNodeWithText("Back to Machine").performClick()
        }
    }

    @Test fun onlyTheLiveCardHasTheMachineDetailsButton() {
        show()
        compose.onNodeWithText("Machine details").assertExists()
        for (view in listOf(MachineView.Stale(last), unreachable(last = last))) {
            compose.runOnIdle { current.value = view }
            compose.onNodeWithText("Machine details").assertDoesNotExist()
        }
    }

    @Test fun aNewMachineStartsOnItsCard() {
        show()
        openDetail()
        compose.runOnIdle { machine.value = "other" }
        assertOnCard()
    }
}
