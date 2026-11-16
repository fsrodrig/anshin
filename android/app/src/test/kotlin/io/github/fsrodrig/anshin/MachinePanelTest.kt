package io.github.fsrodrig.anshin

import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import kotlin.time.Duration.Companion.seconds
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner

@RunWith(RobolectricTestRunner::class)
class MachinePanelTest {
    @get:Rule val compose = createComposeRule()

    private val now = mutableStateOf(Ages.now)
    private val view = mutableStateOf<MachineView>(MachineView.Connecting)

    private fun show(initial: MachineView) {
        view.value = initial
        compose.setContent {
            AnshinTheme(Theme.Default) { MachinePanel(Machines.laptop, { view.value }, { now.value }, onOpen = {}) }
        }
    }

    private fun shows(text: String) = compose.onNodeWithText(text, useUnmergedTree = true).assertExists()

    private fun lacks(text: String) = compose.onAllNodesWithText(text, substring = true, useUnmergedTree = true).assertCountEquals(0)

    @Test
    fun aLivePanelShowsItsValues() {
        show(live(Machines.laptopPulse))
        shows("laptop")
        shows(breakableAddress("laptop.example.ts.net"))
        shows("Live")
        shows("Updated 0 s ago")
        shows("21%")
        shows("25%")
        shows("41%")
        shows("52 °C")
        shows("64% discharging")
    }

    @Test
    fun everyUnavailableValueSaysSoAndNeverZero() {
        show(live(Pulse(schema = PULSE_SCHEMA)))
        compose.onAllNodesWithText(UNAVAILABLE, useUnmergedTree = true).assertCountEquals(5)
        lacks("0%")
        lacks("0 °C")
    }

    @Test
    fun aBatteryTheAgentCouldNotClassifyIsUnavailableAndNoBatteryHidesTheRow() {
        show(live(Machines.unavailablePulse))
        shows("Battery")
        compose.onAllNodesWithText(UNAVAILABLE, useUnmergedTree = true).assertCountEquals(5)
        compose.runOnIdle { view.value = live(Machines.desktopPulse) }
        lacks("Battery")
    }

    @Test
    fun thePrimaryDiskIsTheOneTheAgentNamedNeverTheFirstListed() {
        val disks = listOf(Filesystem(majMin = "0:40", usedPct = 90.0), Filesystem(majMin = "0:35", usedPct = 41.3))
        show(live(Pulse(schema = PULSE_SCHEMA, filesystems = disks, primaryDisk = "0:35")))
        shows("41%")
        lacks("90%")
        compose.runOnIdle { view.value = live(Pulse(schema = PULSE_SCHEMA, filesystems = disks, primaryDisk = "0:99")) }
        lacks("90%")
        lacks("41%")
        compose.runOnIdle { view.value = live(Pulse(schema = PULSE_SCHEMA, filesystems = disks)) }
        lacks("90%")
    }

    @Test
    fun oldDataIsNeverShownAsCurrent() {
        show(stale(Machines.laptopPulse))
        shows("Stale")
        shows("Last Sample 42 s ago. Not current.")
        lacks("Live")
        compose.runOnIdle { view.value = unreachable(last = received(Machines.laptopPulse)) }
        shows("Unreachable")
        shows("Last Sample 42 s ago. Not current.")
        lacks("Live")
        compose.runOnIdle { now.value = Ages.now + 1.seconds }
        shows("Last Sample 43 s ago. Not current.")
    }

    @Test
    fun withoutAPulseThePanelShowsNoValues() {
        for ((state, line) in listOf(
            MachineView.Forbidden to "Only answers its Owner's untagged devices",
            MachineView.Incompatible to "Update Anshin and the Agent",
            MachineView.Connecting to "Asking for its first Pulse",
            unreachable() to "No Pulse yet",
            unreachable(outsideTailnet = true) to "Not a tailnet address",
            unreachable(httpStatus = 503) to "Can't ask tailscaled",
            unreachable(httpStatus = 500) to "HTTP 500",
        )) {
            if (state === MachineView.Forbidden) show(state) else compose.runOnIdle { view.value = state }
            shows(line)
            lacks("CPU")
            lacks("Memory")
        }
    }

    @Test
    fun onlyALiveMachineShowsACurrentVerdictAndOldOnesAreQuotedAsHistory() {
        val warn = Machines.laptopPulse.copy(verdict = Machines.diskWarn)
        show(live(warn))
        shows("warn")
        shows("/ is 91 % full")
        lacks("Last Verdict")
        compose.runOnIdle { view.value = live(Machines.laptopPulse.copy(verdict = null)) }
        shows("Verdict unavailable")
        compose.runOnIdle { view.value = stale(warn) }
        shows("Last Verdict warn, 42 s ago")
        compose.onAllNodesWithText("warn", useUnmergedTree = true).assertCountEquals(0)
        compose.runOnIdle { view.value = unreachable(last = received(warn)) }
        shows("Last Verdict warn, 42 s ago")
        for (state in listOf(MachineView.Connecting, MachineView.Forbidden, MachineView.Incompatible, unreachable())) {
            compose.runOnIdle { view.value = state }
            lacks("Verdict")
            lacks("warn")
        }
    }
}
