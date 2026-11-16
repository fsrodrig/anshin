package io.github.fsrodrig.anshin

import kotlin.time.TestTimeSource
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class DetailStateTest {
    private val pulse = Pulse(schema = 1, kind = PulseKind.Full, hostname = "machine", cpuPct = 21.4,
        verdict = Verdict(VerdictLevel.Warn, listOf(Reason("memory_low", VerdictLevel.Warn, "Memory available is 8 %"))))
    private val last = LastPulse(pulse, TestTimeSource().markNow())
    private val window = WindowSamples.samples(10)

    @Test
    fun connectingHasNoDetail() {
        assertNull((null as Followed?).detail("machine"))
    }

    @Test
    fun anOkAnswerIsLive() {
        assertEquals(
            MachineDetailState("machine", Connection.Live, last, window),
            Followed(PulseResult.Ok(pulse), window, last).detail("machine"),
        )
    }

    @Test
    fun unreachableWithALastPulseKeepsItAndItsWindow() {
        for (outside in listOf(false, true)) {
            assertEquals(
                MachineDetailState("machine", Connection.Unreachable(outside), last, window),
                Followed(PulseResult.Unreachable(outside), window, last).detail("machine"),
            )
        }
    }

    @Test
    fun unreachableBeforeAnyPulseHasNoDetail() {
        assertNull(Followed(PulseResult.Unreachable(false), null, null).detail("machine"))
    }

    @Test
    fun anAnswerThatWithholdsDataHasNoDetailEvenWithALastPulse() {
        for (result in listOf(PulseResult.Forbidden, PulseResult.Incompatible, PulseResult.Failed(503), PulseResult.Failed(500))) {
            assertNull(result.toString(), Followed(result, window, last).detail("machine"))
        }
    }

    @Test
    fun onlyLiveShowsTheVerdict() {
        assertEquals(pulse.verdict, MachineDetailState("machine", Connection.Live, last, window).currentVerdict)
        for (connection in listOf(Connection.Stale, Connection.Unreachable(false), Connection.Unreachable(true))) {
            assertNull(connection.toString(), MachineDetailState("machine", connection, last, window).currentVerdict)
        }
    }
}
