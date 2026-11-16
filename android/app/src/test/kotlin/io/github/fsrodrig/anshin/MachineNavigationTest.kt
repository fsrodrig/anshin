package io.github.fsrodrig.anshin

import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToKey
import androidx.test.espresso.Espresso
import kotlin.time.TestTimeSource
import kotlin.time.Duration.Companion.minutes
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class MachineNavigationTest {
    @get:Rule val compose = createComposeRule()

    private val pulse = DetailSamples.pulse
    private val last = LastPulse(pulse, TestTimeSource().markNow())
    private val live = Followed(PulseResult.Ok(pulse), DetailSamples.window, last)
    private val machine = mutableStateOf(DetailSamples.MACHINE)
    private val current = mutableStateOf<Followed?>(live)

    private fun show() {
        compose.setContent {
            AnshinTheme(Theme.Default) {
                val followed = current.value
                MachineScreens(machine.value, followed) { open ->
                    PulseLayout(machine.value, onInputChange = {}, onWatch = {}) {
                        PulseView(followed?.result, machine.value, followed?.window, onDetail = open)
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

    @Test fun theDetailKeepsTheLastValuesAgedWhenTheMachineBecomesUnreachable() {
        show()
        openDetail()
        compose.onNodeWithText("Live").assertIsDisplayed()
        compose.onNodeWithText("ok").assertIsDisplayed()
        compose.runOnIdle { current.value = Followed(PulseResult.Unreachable(outsideTailnet = false), live.window, last) }
        compose.onNodeWithText("Unreachable").assertIsDisplayed()
        compose.onNodeWithText("Live").assertDoesNotExist()
        compose.onNodeWithText("ok").assertDoesNotExist()
        compose.onNodeWithText("No current Verdict").assertIsDisplayed()
        compose.onNodeWithTag("machine-detail").performScrollToKey("cpu-now")
        compose.onNodeWithText("70.2 %").assertIsDisplayed()
    }

    @Test fun anAnswerThatWithholdsDataShowsItsMessageInsideTheDetail() {
        show()
        openDetail()
        val messages = mapOf(
            PulseResult.Forbidden to "Forbidden",
            PulseResult.Incompatible to "Incompatible",
            PulseResult.Failed(503) to "Agent can't reach tailscaled",
            PulseResult.Failed(500) to "No Pulse",
        )
        for ((result, title) in messages) {
            compose.runOnIdle { current.value = Followed(result, live.window, last) }
            compose.onNodeWithText(title).assertIsDisplayed()
            compose.onNodeWithText("Back to Machine").assertIsDisplayed()
            compose.onNodeWithTag("machine-detail").assertDoesNotExist()
            compose.onNodeWithText("Watch").assertDoesNotExist()
        }
        compose.runOnIdle { current.value = live }
        compose.onNodeWithTag("machine-detail").assertIsDisplayed()
        compose.onNodeWithText("Live").assertIsDisplayed()
    }

    @Test fun unreachableBeforeAnyPulseAndConnectingShowTheirMessageInsideTheDetail() {
        show()
        openDetail()
        compose.runOnIdle { current.value = Followed(PulseResult.Unreachable(outsideTailnet = false), null, null) }
        compose.onNodeWithText("Unreachable").assertIsDisplayed()
        compose.onNodeWithTag("machine-detail").assertDoesNotExist()
        compose.runOnIdle { current.value = null }
        compose.onNodeWithText("Connecting").assertIsDisplayed()
        compose.onNodeWithText("Back to Machine").assertIsDisplayed()
    }

    @Test fun aFreshPulseImmediatelyReplacesThePreviousAge() {
        val clock = TestTimeSource()
        val old = LastPulse(pulse, clock.markNow())
        clock += 2.minutes
        current.value = live.copy(last = old)
        show()
        openDetail()
        compose.onNodeWithText(" · last Pulse 2 min ago").assertIsDisplayed()
        compose.runOnIdle { current.value = live.copy(last = LastPulse(pulse, clock.markNow())) }
        compose.onNodeWithText(" · last Pulse 0 s ago").assertIsDisplayed()
    }

    @Test fun tappingTheLiveCardOpensTheDetail() {
        show()
        compose.onNodeWithTag("live-card").performClick()
        compose.onNodeWithText("Back to Machine").assertIsDisplayed()
    }

    @Test fun aNewMachineStartsOnItsCard() {
        show()
        openDetail()
        compose.runOnIdle { machine.value = "other" }
        assertOnCard()
    }
}
