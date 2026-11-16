package io.github.fsrodrig.anshin

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Test

class TopLineTest {
    private val ok = Verdict(VerdictLevel.Ok, emptyList())
    private val warn = Verdict(VerdictLevel.Warn, listOf(Reason("filesystem_full", VerdictLevel.Warn, "/ is 91.2 % full")))
    private val crit = Verdict(VerdictLevel.Crit, listOf(
        Reason("memory_low", VerdictLevel.Crit, "Memory available is 3 %"),
        Reason("filesystem_full", VerdictLevel.Warn, "/ is 91.2 % full"),
    ))
    private val unknown = Verdict(VerdictLevel.Other("dire"), listOf(Reason("fan_stalled", null, "A fan stopped")))

    private fun live(name: String, verdict: Verdict?) = WatchedMachine(name, live(Pulse(schema = 1, verdict = verdict)))

    @Test fun noMachineWatchedHasNoTopLine() = assertNull(topLine(emptyList()))

    @Test fun oneOkMachineIsAllMachinesOk() =
        assertEquals(TopLine(Headline.AllOk, emptyList()), topLine(listOf(live("a", ok))))

    @Test fun oneWarnMachineIsNamedWithItsVerdict() =
        assertEquals(TopLine(Headline.Worst("a", warn), emptyList()), topLine(listOf(live("a", warn))))

    @Test fun oneCritMachineIsNamedWithItsVerdict() =
        assertEquals(TopLine(Headline.Worst("a", crit), emptyList()), topLine(listOf(live("a", crit))))

    @Test fun oneUnknownWordIsShownAsItselfNotOk() =
        assertEquals(TopLine(null, emptyList(), listOf(Headline.Worst("a", unknown))), topLine(listOf(live("a", unknown))))

    @Test fun aMachineWithoutACurrentVerdictIsNamed() {
        val withoutVerdict = listOf(
            live("a", null),
            WatchedMachine("a", MachineView.Connecting),
            WatchedMachine("a", MachineView.Forbidden),
            WatchedMachine("a", MachineView.Incompatible),
            WatchedMachine("a", unreachable()),
            WatchedMachine("a", unreachable(httpStatus = 503)),
            // The last data's Verdict is history: never listed as current, never ranked.
            WatchedMachine("a", stale(Pulse(schema = 1, verdict = crit))),
            WatchedMachine("a", unreachable(last = received(Pulse(schema = 1, verdict = crit)))),
        )
        for (machine in withoutVerdict) {
            assertEquals(machine.toString(), TopLine(null, listOf("a")), topLine(listOf(machine)))
        }
    }

    @Test fun theWorstOfTwoMachinesWinsAndIsNamed() =
        assertEquals(TopLine(Headline.Worst("b", warn), emptyList()), topLine(listOf(live("a", ok), live("b", warn))))

    @Test fun twoOkMachinesAreAllMachinesOk() =
        assertEquals(TopLine(Headline.AllOk, emptyList()), topLine(listOf(live("a", ok), live("b", ok))))

    @Test fun oneUnavailableAndOneOkNeverSaysAllMachinesOk() {
        val line = topLine(listOf(WatchedMachine("a", unreachable()), live("b", ok)))
        assertEquals(TopLine(Headline.Worst("b", ok), listOf("a")), line)
        assertNotEquals(Headline.AllOk, line?.headline)
    }

    @Test fun critBeatsWarnInEitherOrder() {
        assertEquals(Headline.Worst("b", crit), topLine(listOf(live("a", warn), live("b", crit)))?.headline)
        assertEquals(Headline.Worst("a", crit), topLine(listOf(live("a", crit), live("b", warn)))?.headline)
    }

    @Test fun unknownWordsAreListedSeparatelyFromEveryKnownLevel() {
        for (known in listOf(ok, warn, crit)) {
            for (machines in listOf(listOf(live("a", unknown), live("b", known)), listOf(live("b", known), live("a", unknown)))) {
                assertEquals(TopLine(Headline.Worst("b", known), emptyList(), listOf(Headline.Worst("a", unknown))), topLine(machines))
            }
        }
    }

    @Test fun onlyUnknownWordsHaveNoRankedHeadline() =
        assertEquals(
            TopLine(null, emptyList(), listOf(Headline.Worst("a", unknown), Headline.Worst("b", unknown))),
            topLine(listOf(live("a", unknown), live("b", unknown))),
        )

    @Test fun theWorstStillNamesEveryMachineWithoutAVerdict() =
        assertEquals(
            TopLine(Headline.Worst("b", crit), listOf("a", "c")),
            topLine(listOf(WatchedMachine("a", MachineView.Connecting), live("b", crit), live("c", null))),
        )
}
