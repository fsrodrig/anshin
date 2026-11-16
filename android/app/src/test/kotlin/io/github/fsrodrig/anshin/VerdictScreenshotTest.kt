package io.github.fsrodrig.anshin

import androidx.compose.runtime.MutableState
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import java.io.File
import okhttp3.OkHttpClient
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.ParameterizedRobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

private const val MACHINE = "machine"

@RunWith(ParameterizedRobolectricTestRunner::class)
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(qualifiers = "w360dp-h640dp-xhdpi")
class VerdictScreenshotTest(private val theme: Theme) {
    @get:Rule val compose = createComposeRule()

    private val machine = Pulse(
        schema = 1, kind = PulseKind.Full, hostname = MACHINE, cpuPct = 21.0, cpuTempC = 52.1, memUsedPct = 40.0,
        swapTotalBytes = 2048, swapUsedBytes = 0, battery = BatteryReport.NoBattery,
    )
    private val ok = Verdict(VerdictLevel.Ok, emptyList())
    private val warn = Verdict(VerdictLevel.Warn, listOf(Reason("filesystem_full", VerdictLevel.Warn, "/ is 91.2 % full")))
    private val crit = Verdict(VerdictLevel.Crit, listOf(
        Reason("memory_low", VerdictLevel.Crit, "Memory available is 3 %"),
        Reason("filesystem_full", VerdictLevel.Warn, "/ is 91.2 % full"),
    ))

    private fun show(result: PulseResult?): MutableState<PulseResult?> {
        val current = mutableStateOf(result)
        compose.setContent { AnshinTheme(theme) {
            PulseLayout(MACHINE, onInputChange = {}, onWatch = {}, topLine = topLine(listOf(WatchedMachine(MACHINE, current.value)))) {
                PulseView(current.value, MACHINE)
            }
        } }
        return current
    }

    private fun future(): Pulse {
        val dir = File(checkNotNull(System.getProperty("anshin.contractFixtures")) { "Gradle sets anshin.contractFixtures" })
        return checkNotNull(decodePulse(File(dir, "future.json").readText()))
    }

    @Test fun ok() {
        show(PulseResult.Ok(machine.copy(verdict = ok)))
        compose.onNodeWithText("ALL MACHINES OK", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("ok", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        compose.onRoot().captureFrame("verdict-ok.${theme.key}.png")
    }

    @Test fun warnWithOneReason() {
        show(PulseResult.Ok(machine.copy(verdict = warn)))
        compose.onNodeWithText("WARN · $MACHINE", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("warn", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        compose.onRoot().captureFrame("verdict-warn.${theme.key}.png")
    }

    @Test fun critWithTwoReasons() {
        show(PulseResult.Ok(machine.copy(verdict = crit)))
        compose.onNodeWithText("CRIT · $MACHINE", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("crit", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        compose.onAllNodesWithText("Memory available is 3 %", useUnmergedTree = true).assertCountEquals(2)
        compose.onNodeWithText("/ is 91.2 % full", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        compose.onRoot().captureFrame("verdict-crit.${theme.key}.png")
    }

    @Test fun theFutureFixturesUnknownReasonRendersAsItsMessageNeverAsACrash() {
        show(PulseResult.Ok(future()))
        compose.onNodeWithText("WARN · $MACHINE", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("warn", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        compose.onAllNodesWithText("A fan stopped", useUnmergedTree = true)[1].performScrollTo().assertIsDisplayed()
        compose.onRoot().captureFrame("verdict-future.${theme.key}.png")
    }

    @Test fun noCurrentVerdict() {
        show(PulseResult.Ok(machine))
        compose.onNodeWithText("No current Verdict from $MACHINE", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("Verdict unavailable", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("ALL MACHINES OK", useUnmergedTree = true).assertDoesNotExist()
        compose.onRoot().captureFrame("verdict-unavailable.${theme.key}.png")
    }

    @Test fun theVerdictIsShownInTextAndColourNeverColourAlone() {
        val current = mutableStateOf(machine)
        compose.setContent { AnshinTheme(theme) {
            PulseLayout(MACHINE, onInputChange = {}, onWatch = {}) { PulseView(PulseResult.Ok(current.value), MACHINE) }
        } }
        val unknown = Verdict(VerdictLevel.Other("dire"), listOf(Reason("fan_stalled", VerdictLevel.Other("grave"), null), Reason()))
        for (verdict in listOf(ok, warn, crit, unknown)) {
            compose.runOnIdle { current.value = machine.copy(verdict = verdict) }
            compose.onNodeWithText(verdict.level.word, useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        }
        compose.onNodeWithText("fan_stalled", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("A Reason without a message", useUnmergedTree = true).performScrollTo().assertIsDisplayed()
    }

    @Test fun unknownTopLineIsUnrankedAndKeepsItsReason() {
        val unknown = Verdict(VerdictLevel.Other("dire"), listOf(Reason(message = "A fan stopped")))
        val current = show(PulseResult.Ok(machine.copy(verdict = unknown)))
        val headline = compose.onNodeWithText("dire · $MACHINE", useUnmergedTree = true).assertIsDisplayed()
            .fetchSemanticsNode().config[SemanticsProperties.Text].single()
        assertTrue(headline.spanStyles.any { it.start == 0 && it.end >= 4 && it.item.fontFamily != null })
        compose.onNodeWithText("ALL MACHINES OK", useUnmergedTree = true).assertDoesNotExist()
        compose.onRoot().captureFrame("verdict-unknown.${theme.key}.png")
        for ((reason, text) in listOf(Reason(code = "fan_stalled") to "fan_stalled", Reason() to "A Reason without a message")) {
            compose.runOnIdle {
                current.value = PulseResult.Ok(machine.copy(verdict = Verdict(VerdictLevel.Other("dire"), listOf(reason))))
            }
            compose.onAllNodesWithText(text, useUnmergedTree = true).assertCountEquals(2)
            compose.onAllNodesWithText(text, useUnmergedTree = true)[0].assertIsDisplayed()
        }
    }

    @Test fun aVerdictIsWithheldUnlessTheMachineIsLive() {
        val current = show(PulseResult.Ok(machine.copy(verdict = crit)))
        compose.onNodeWithText("CRIT · $MACHINE", useUnmergedTree = true).assertIsDisplayed()
        for (result in listOf(null, PulseResult.Forbidden, PulseResult.Incompatible, PulseResult.Unreachable(outsideTailnet = false), PulseResult.Failed(503))) {
            compose.runOnIdle { current.value = result }
            compose.onNodeWithText("No current Verdict from $MACHINE", useUnmergedTree = true).assertIsDisplayed()
            compose.onNodeWithText("CRIT · $MACHINE", useUnmergedTree = true).assertDoesNotExist()
            compose.onNodeWithText("crit", useUnmergedTree = true).assertDoesNotExist()
        }
    }

    @Test fun theTopLineAppearsOnlyOnceAMachineIsWatched() {
        compose.setContent { AnshinTheme(theme) { PulseScreen(AgentLink(OkHttpClient())) } }
        compose.onNode(hasSetTextAction()).performTextInput(MACHINE)
        compose.onNodeWithText("No current Verdict from $MACHINE", useUnmergedTree = true).assertDoesNotExist()
        compose.onNodeWithText("Watch", useUnmergedTree = true).performClick()
        compose.onNodeWithText("No current Verdict from $MACHINE", useUnmergedTree = true).assertIsDisplayed()
    }

    companion object {
        @JvmStatic
        @ParameterizedRobolectricTestRunner.Parameters(name = "{0}")
        fun themes() = Theme.entries.map { arrayOf<Any>(it) }
    }
}
