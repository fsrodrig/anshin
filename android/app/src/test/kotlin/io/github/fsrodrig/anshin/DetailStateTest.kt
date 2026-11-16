package io.github.fsrodrig.anshin

import kotlin.time.Duration.Companion.seconds
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** The detail draws what the Watcher says: Live, Stale and Unreachable with its last data; nothing for the rest. */
class DetailStateTest {
    private val pulse = Pulse(schema = 1, kind = PulseKind.Full, hostname = "machine", cpuPct = 21.4,
        verdict = Verdict(VerdictLevel.Warn, listOf(Reason("memory_low", VerdictLevel.Warn, "Memory available is 8 %"))))
    private val window = WindowSamples.samples(10)
    private val last = received(pulse, window)

    @Test
    fun liveIsLiveWithItsWindow() {
        val state = MachineView.Live(last).detail("machine")!!
        assertEquals(MachineDetailState("machine", Connection.Live, last), state)
        assertEquals(window, state.window)
    }

    @Test
    fun staleFromTheWatcherKeepsItsLastData() {
        assertEquals(MachineDetailState("machine", Connection.Stale, last), stale(pulse, window).detail("machine"))
    }

    @Test
    fun unreachableWithLastDataCarriesEverythingTheCardSays() {
        val views = listOf(
            unreachable(last = last),
            unreachable(last = last, outsideTailnet = true),
            unreachable(last = last, httpStatus = 503, vpnHint = true, retryIn = 8.seconds),
            unreachable(last = last, httpStatus = 500),
        )
        for (view in views) {
            assertEquals(
                view.toString(),
                MachineDetailState("machine", Connection.Unreachable(view.retryAt, view.vpnHint, view.outsideTailnet, view.httpStatus), last),
                view.detail("machine"),
            )
        }
    }

    @Test
    fun aViewWithoutDataHasNoDetail() {
        for (view in listOf(MachineView.Connecting, MachineView.Forbidden, MachineView.Incompatible, unreachable(), unreachable(httpStatus = 503))) {
            assertNull(view.toString(), view.detail("machine"))
        }
    }
}
