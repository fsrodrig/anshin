// currentTime and runCurrent are the virtual clock these tests are about; kotlinx-coroutines-test still marks them experimental.
@file:OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)

package io.github.fsrodrig.anshin

import kotlin.random.Random
import kotlin.time.ComparableTimeMark
import kotlin.time.Duration.Companion.seconds
import kotlinx.coroutines.delay
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.testTimeSource
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Every u is 0: each delay is its whole step, so times are exact. */
private object NoJitter : Random() {
    override fun nextBits(bitCount: Int) = 0
    override fun nextDouble() = 0.0
}

private class ScriptedJitter(vararg u: Double) : Random() {
    private val next = ArrayDeque(u.toList())
    override fun nextBits(bitCount: Int) = 0
    override fun nextDouble() = next.removeFirst()
}

private class Seen(val views: MutableList<Pair<Long, MachineView>>, val job: Job)

private class Harness(val scope: TestScope, random: Random, vpn: () -> Boolean) {
    val fake = FakeAgent(scope.testScheduler)
    val clock = scope.testTimeSource
    private val zero = clock.markNow()
    val watcher = Watcher(fake, clock, random, vpn)

    fun ms(mark: ComparableTimeMark) = (mark - zero).inWholeMilliseconds

    fun flow(machine: String) = watcher.watch(fake.url(machine))

    fun follow(flow: Flow<MachineView>): Seen {
        val views = mutableListOf<Pair<Long, MachineView>>()
        val job = scope.backgroundScope.launch { flow.collect { views += scope.testScheduler.currentTime to it } }
        scope.testScheduler.runCurrent()
        return Seen(views, job)
    }

    fun follow(machine: String) = follow(flow(machine))

    /** Runs everything due up to and at [ms] on the virtual clock. */
    fun at(ms: Long) {
        scope.advanceTimeBy(ms - scope.testScheduler.currentTime)
        scope.runCurrent()
    }

    /** A view and the times it carries, comparable across runs on different clocks. */
    fun describe(view: MachineView): String = when (view) {
        MachineView.Connecting -> "Connecting"
        is MachineView.Live -> "Live@${ms(view.received.at)}"
        is MachineView.Stale -> "Stale@${ms(view.since)}"
        is MachineView.Unreachable -> "Unreachable@${view.last?.let { ms(it.at) }} retry@${ms(view.retryAt)}"
        MachineView.Forbidden -> "Forbidden"
        MachineView.Incompatible -> "Incompatible"
    }
}

private fun watching(random: Random = NoJitter, vpn: () -> Boolean = { false }, test: suspend Harness.() -> Unit) =
    runTest { Harness(this, random, vpn).test() }

private fun MachineView.kind() = this::class.simpleName!!

/** Consecutive views of the same kind collapse to one, so the list reads as the transitions. */
private fun Seen.kinds() = views.map { it.second.kind() }.fold(listOf<String>()) { acc, k -> if (acc.lastOrNull() == k) acc else acc + k }

private fun List<Call>.gaps() = zipWithNext { a, b -> b.startMs - a.startMs }

/** How long the Watcher waited after each failed call before the next. */
private fun List<Call>.waits() = zipWithNext { a, b -> b.startMs - a.endMs!! }

/** The calls that ended at or after [ms]: a stop is noticed by the long-poll held at that moment. */
private fun FakeAgent.callsEndingFrom(machine: String, ms: Long) = callsTo(machine).filter { (it.endMs ?: Long.MAX_VALUE) >= ms }

private fun Seen.lastLive() = views.last { it.second is MachineView.Live }.second as MachineView.Live

class WatcherTest {
    // AC 1: Live → Stale → Unreachable → Live.

    @Test
    fun liveThenStaleThenUnreachableThenLiveAgain() = watching {
        val machine = fake.machine("a")
        val seen = follow("a")
        at(20_000)
        // The newest Sample before sleep landed at 19.5 s, answering the long-poll that started at 17.5 s.
        val beforeSleep = seen.lastLive().received
        assertEquals(19_500, ms(beforeSleep.at))
        machine.asleep()

        at(60_000)
        machine.answering()
        at(70_000)

        assertEquals(listOf("Connecting", "Live", "Stale", "Unreachable", "Live"), seen.kinds())
        val (staleAt, stale) = seen.views.first { it.second is MachineView.Stale }
        assertEquals(29_500, staleAt)
        assertEquals(beforeSleep.at, (stale as MachineView.Stale).since)

        val (unreachableAt, unreachable) = seen.views.first { it.second is MachineView.Unreachable }
        val hung = fake.callsTo("a").first { it.startMs == 19_500L }
        assertEquals(10.seconds, hung.wait)
        assertEquals("the held long-poll fails at its read timeout, 5 s + wait", 34_500L, hung.endMs)
        assertEquals(34_500, unreachableAt)
        unreachable as MachineView.Unreachable
        assertEquals(beforeSleep, unreachable.last)
        assertEquals(beforeSleep.window, unreachable.last!!.window)
        val retry = fake.callsTo("a").first { it.startMs > 34_500 }
        assertEquals(retry.startMs, ms(unreachable.retryAt))

        val firstOk = fake.callsTo("a").first { it.startMs > 34_500 && it.result is PulseResult.Ok }
        val (liveAt, live) = seen.views.first { it.first > 34_500 && it.second is MachineView.Live }
        assertEquals(firstOk.endMs, liveAt)
        val window = (live as MachineView.Live).received.window!!
        assertEquals(beforeSleep.window!!.cursor.substringBefore('.'), window.cursor.substringBefore('.'))
        assertEquals("the delta appends to the held Window with a Gap", beforeSleep.window.window.boottimeMs,
            window.window.boottimeMs.take(beforeSleep.window.window.rows))
        assertTrue(window.window.boottimeMs.drop(beforeSleep.window.window.rows).contains(null))
    }

    @Test
    fun retryAtTheStalenessDeadlineStopsShowingLiveWhileTheRequestHangs() = runTest {
        val fake = FakeAgent(testScheduler)
        var calls = 0
        val link = AgentLink { endpoint, after, wait ->
            when (calls++) {
                0 -> fake.pulse(endpoint, after, wait)
                1 -> {
                    delay(8.seconds)
                    PulseResult.Unreachable(outsideTailnet = false)
                }
                else -> {
                    delay(15.seconds)
                    PulseResult.Unreachable(outsideTailnet = false)
                }
            }
        }
        val views = mutableListOf<Pair<Long, MachineView>>()
        backgroundScope.launch {
            Watcher(link, testTimeSource, NoJitter).watch(fake.url("a")).collect {
                views += testScheduler.currentTime to it
            }
        }
        runCurrent()
        assertTrue(views.last().second is MachineView.Live)
        advanceTimeBy(9_000)
        runCurrent()
        assertTrue(views.last().second is MachineView.Live)
        advanceTimeBy(1_000)
        runCurrent()
        assertEquals(3, calls)
        assertTrue(views.last().second is MachineView.Unreachable)
        advanceTimeBy(14_999)
        runCurrent()
        assertTrue(views.last().second is MachineView.Unreachable)
        assertTrue(views.none { (at, view) -> at >= 10_000 && view is MachineView.Live })
    }

    @Test
    fun aFailureWhileTheDataIsFreshStaysLive() = watching {
        val machine = fake.machine("a")
        val seen = follow("a")
        at(20_000)
        machine.stopped()
        at(23_000)
        machine.answering()
        at(60_000)
        assertTrue(fake.callsTo("a").any { it.result is PulseResult.Unreachable })
        assertEquals(listOf("Connecting", "Live"), seen.kinds())
    }

    @Test
    fun anAgentThatStopsIsUnreachableOnceItsDataIsStale() = watching {
        val machine = fake.machine("a")
        val seen = follow("a")
        at(20_000)
        val last = seen.lastLive().received
        machine.stopped()
        at(60_000)
        assertEquals(listOf("Connecting", "Live", "Unreachable"), seen.kinds())
        val (at, view) = seen.views.first { it.second is MachineView.Unreachable }
        assertEquals(ms(last.at) + 10_000, at)
        assertEquals(last, (view as MachineView.Unreachable).last)
        assertFalse(view.vpnHint)
    }

    // AC 1: 403 → Forbidden, never in a tight loop.

    @Test
    fun forbiddenIsAskedAgainAtMostOnceEvery30Seconds() {
        watching(random = Random(3)) {
            fake.machine("a").forbids()
            val seen = follow("a")
            at(600_000)
            assertEquals(listOf("Connecting", "Forbidden"), seen.kinds())
            val calls = fake.callsTo("a")
            assertTrue("${calls.size} calls in 10 minutes", calls.size <= 21)
            assertTrue(calls.gaps().toString(), calls.gaps().all { it in 30_001..60_000 })
        }
        watching {
            fake.machine("a").forbids()
            follow("a")
            at(600_000)
            assertEquals(List(10) { 60_000L }, fake.callsTo("a").gaps())
        }
    }

    @Test
    fun forbiddenAfterLiveWithholdsItsData() = watching {
        val machine = fake.machine("a")
        val seen = follow("a")
        at(20_000)
        machine.forbids()
        at(30_000)
        assertEquals(listOf("Connecting", "Live", "Forbidden"), seen.kinds())
        assertEquals(MachineView.Forbidden, seen.views.last().second)
    }

    @Test
    fun aFixedPolicyIsLiveWithinAMinute() = watching(random = Random(5)) {
        val machine = fake.machine("a")
        machine.forbids()
        val seen = follow("a")
        at(100_000)
        machine.answering()
        at(160_000)
        val (liveAt, _) = seen.views.first { it.second is MachineView.Live }
        assertTrue("Live at $liveAt", liveAt <= 160_000)
        assertEquals(listOf("Connecting", "Forbidden", "Live"), seen.kinds())
    }

    // AC 1: an unknown schema → Incompatible.

    @Test
    fun anUnknownSchemaIsIncompatibleAndAskedAgainSlowly() = watching(random = Random(9)) {
        val machine = fake.machine("a")
        val seen = follow("a")
        at(10_000)
        machine.speaks(schema = 2)
        at(200_000)
        val refused = fake.callsTo("a").filter { it.result == PulseResult.Incompatible }
        assertTrue(refused.size >= 3)
        assertTrue(refused.waits().toString(), refused.waits().all { it in 30_001..60_000 })
        assertTrue("an unreadable Pulse drops the Window", refused.drop(1).all { it.after == null })
        assertEquals(MachineView.Incompatible, seen.views.last().second)

        machine.speaks(schema = PULSE_SCHEMA)
        at(260_000)
        assertEquals(listOf("Connecting", "Live", "Incompatible", "Live"), seen.kinds())
    }

    // AC 1: backoff is capped and jittered.

    @Test
    fun backoffDoublesFromOneSecondUpTo16Seconds() = watching {
        val machine = fake.machine("a")
        follow("a")
        at(10_000)
        machine.stopped()
        at(200_000)
        val waits = fake.callsEndingFrom("a", 10_000).waits()
        assertEquals(listOf(1_000L, 2_000, 4_000, 8_000, 16_000, 16_000, 16_000), waits.take(7))
        assertTrue(waits.all { it <= 16_000 })
    }

    @Test
    fun backoffIsJittered() {
        watching(random = ScriptedJitter(0.5, 0.5, 0.25, 0.5, 0.75, 0.5, 0.25, 0.0)) {
            val machine = fake.machine("a")
            follow("a")
            at(10_000)
            machine.stopped()
            // The held call fails at 10.1 s and the seventh wait ends at 57.85 s. The call then is the eighth failure,
            // which draws one more u, so the script ends with a spare.
            at(57_850)
            assertEquals(listOf(750L, 1_500, 3_500, 6_000, 10_000, 12_000, 14_000), fake.callsEndingFrom("a", 10_000).waits().take(7))
        }
        watching(random = Random(42)) {
            val machine = fake.machine("a")
            follow("a")
            at(10_000)
            machine.stopped()
            at(400_000)
            val gaps = fake.callsEndingFrom("a", 10_000).waits()
            val steps = listOf(1_000L, 2_000, 4_000, 8_000) + List(gaps.size - 4) { 16_000L }
            gaps.zip(steps).forEach { (gap, step) -> assertTrue("wait $gap for step $step", gap in step / 2 + 1..step) }
            val capped = gaps.drop(4)
            assertTrue(capped.toString(), capped.size > 5 && capped.distinct().size > 1)
        }
    }

    @Test
    fun twoMachinesFailingTogetherDoNotRetryInStep() = watching(random = Random(7)) {
        follow("a")
        follow("b")
        at(10_000)
        fake.machine("a").stopped()
        fake.machine("b").stopped()
        at(120_000)
        val a = fake.callsTo("a").filter { it.startMs >= 10_000 }.map { it.startMs }.drop(1)
        val b = fake.callsTo("b").filter { it.startMs >= 10_000 }.map { it.startMs }.drop(1)
        assertTrue(a.isNotEmpty())
        assertNotEquals(a, b)
    }

    // AC 1: one Machine failing never delays another.

    @Test
    fun oneMachineFailingNeverDelaysAnother() {
        fun run(withA: Boolean): Pair<List<String>, List<Call>> {
            var out: Pair<List<String>, List<Call>>? = null
            watching(random = Random(11)) {
                val a = fake.machine("a")
                if (withA) follow("a")
                val seen = follow("b")
                at(10_000)
                a.asleep()
                at(60_000)
                a.forbids()
                at(120_000)
                a.stopped()
                at(180_000)
                out = seen.views.map { (t, v) -> "$t ${describe(v)}" } to fake.callsTo("b").map { it.copy(result = null) }
                if (withA) assertTrue(fake.callsTo("a").size > 5)
            }
            return out!!
        }
        assertEquals(run(withA = false), run(withA = true))
    }

    @Test
    fun backoffStartsOverAfterAnAnswer() = watching {
        val machine = fake.machine("a")
        follow("a")
        at(10_000)
        machine.stopped()
        at(40_000)
        machine.answering()
        at(80_000)
        machine.stopped()
        at(120_000)
        assertEquals(listOf(1_000L, 2_000, 4_000), fake.callsEndingFrom("a", 80_000).waits().take(3))
    }

    @Test
    fun anAgentThatIgnoresWaitIsAskedAtMostOncePerSecond() {
        watching {
            fake.machine("a").ignoresWait()
            follow("a")
            at(59_999)
            assertTrue(fake.callsTo("a").size <= 61)
            assertTrue(fake.callsTo("a").gaps().all { it >= 1_000 })
        }
        watching {
            fake.machine("a").sendsMalformedWindows()
            val seen = follow("a")
            at(59_999)
            val calls = fake.callsTo("a")
            assertTrue(calls.all { it.after == null })
            assertTrue(calls.size in 2..30)
            assertTrue(calls.gaps().all { it >= 2_000 })
            assertNull(seen.lastLive().received.window)
        }
    }

    // Merging full and delta Pulses.

    @Test
    fun theFirstRequestAsksForTheWholeWindowThenLongPollsWithItsCursor() = watching {
        val seen = follow("a")
        at(2_000)
        val calls = fake.callsTo("a")
        assertNull(calls[0].after)
        assertNull(calls[0].wait)
        assertEquals("0000000000000001.3", calls[1].after)
        assertEquals(10.seconds, calls[1].wait)
        assertEquals(1_000, calls[1].startMs)
        assertEquals(1_500L, calls[1].endMs)
        val window = seen.lastLive().received.window!!
        assertEquals("0000000000000001.4", window.cursor)
        assertEquals(4, window.window.rows)
    }

    @Test
    fun aFailedRequestKeepsTheWindowAndItsCursor() = watching {
        val machine = fake.machine("a")
        val seen = follow("a")
        at(10_000)
        val before = seen.lastLive().received.window!!
        machine.status(500)
        at(10_100)
        machine.answering()
        at(13_000)
        val calls = fake.callsTo("a")
        val failed = calls.indexOfFirst { it.result == PulseResult.Failed(500) }
        assertEquals(before.cursor, calls[failed + 1].after)
        val after = seen.lastLive().received.window!!
        assertEquals(before.window.boottimeMs, after.window.boottimeMs.take(before.window.rows))
        assertTrue(after.window.rows > before.window.rows)
    }

    @Test
    fun anInconsistentDeltaDropsTheWindowSoTheNextRequestAsksForAllOfIt() = watching {
        val machine = fake.machine("a")
        val seen = follow("a")
        at(5_000)
        machine.sendsAnInconsistentDelta()
        at(10_000)
        val calls = fake.callsTo("a")
        val bad = calls.indexOfFirst { (it.endMs ?: 0) >= 5_000 && it.result is PulseResult.Ok }
        assertNull(calls[bad + 1].after)
        assertEquals(calls.last().after, seen.lastLive().received.window!!.cursor)
    }

    @Test
    fun anAgentRestartReplacesTheWindow() = watching {
        val machine = fake.machine("a")
        val seen = follow("a")
        at(10_000)
        machine.restarts()
        at(20_000)
        val window = seen.lastLive().received.window!!
        assertTrue(window.cursor, window.cursor.startsWith("0000000000000002."))
        assertTrue(window.window.boottimeMs.all { it != null && it > FakeAgent.BOOT_OFFSET_MS + 10_000 })
    }

    // AC 3: Stale data is never shown as current.

    @Test
    fun anAgentThatAnswersWithoutNewSamplesGoesStale() = watching {
        val machine = fake.machine("a")
        val seen = follow("a")
        at(10_000)
        val last = seen.lastLive().received
        machine.frozen()
        at(60_000)
        assertEquals(listOf("Connecting", "Live", "Stale"), seen.kinds())
        val (staleAt, stale) = seen.views.first { it.second is MachineView.Stale }
        assertEquals(ms(last.at) + 10_000, staleAt)
        assertEquals(last.at, (stale as MachineView.Stale).since)
        val held = fake.callsTo("a").filter { it.startMs > ms(last.at) }
        assertTrue(held.size >= 4)
        assertTrue("still answered", held.dropLast(1).all { it.result is PulseResult.Ok && it.endMs!! - it.startMs == 10_000L })
    }

    @Test
    fun collectingAgainAfterTheAppWasHiddenStartsStaleNeverLive() = watching {
        val flow = flow("a")
        val first = follow(flow)
        at(10_000)
        val last = first.lastLive().received
        first.job.cancel()
        at(310_000)
        val callsBefore = fake.callsTo("a").size
        val second = follow(flow)
        val (shownAt, shown) = second.views.first()
        assertEquals(310_000, shownAt)
        assertEquals(MachineView.Stale(last), shown)
        val resumed = fake.callsTo("a")[callsBefore]
        assertEquals(last.window!!.cursor, resumed.after)
        at(311_000)
        assertEquals(listOf("Stale", "Live"), second.kinds())
    }

    @Test
    fun collectingAgainWithinTenSecondsStartsLive() = watching {
        val flow = flow("a")
        val first = follow(flow)
        at(10_000)
        val last = first.lastLive().received
        first.job.cancel()
        at(15_000)
        val second = follow(flow)
        assertEquals(MachineView.Live(last), second.views.first().second)
    }

    // AC 2: nothing is asked while not collected.

    @Test
    fun cancellingCollectionCancelsTheHeldRequestAndAsksNothingMore() = watching {
        val seen = follow("a")
        at(10_000)
        val held = fake.callsTo("a").last()
        assertNull(held.endMs)
        seen.job.cancel()
        scope.runCurrent()
        assertTrue(held.cancelled)
        assertEquals(10_000L, held.endMs)
        val count = fake.calls.size
        at(610_000)
        assertEquals(count, fake.calls.size)
    }

    // AC 4: the VPN hint is advisory.

    @Test
    fun theVpnHintNeverChangesWhatTheWatcherAsks() {
        fun run(vpnLooksOff: Boolean): Pair<List<String>, List<Call>> {
            var out: Pair<List<String>, List<Call>>? = null
            watching(random = Random(13), vpn = { vpnLooksOff }) {
                val machine = fake.machine("a")
                val seen = follow("a")
                at(10_000)
                assertTrue(seen.views.last().second is MachineView.Live)
                machine.stopped()
                at(60_000)
                machine.answering()
                at(90_000)
                assertTrue(seen.views.last().second is MachineView.Live)
                out = seen.kinds() to fake.calls.toList()
            }
            return out!!
        }
        assertEquals(run(vpnLooksOff = false), run(vpnLooksOff = true))
    }

    @Test
    fun theVpnHintIsReadWhenTheMachineBecomesUnreachable() {
        var off = true
        watching(vpn = { off }) {
            val machine = fake.machine("a")
            machine.stopped()
            val seen = follow("a")
            assertTrue((seen.views.last().second as MachineView.Unreachable).vpnHint)
            val count = fake.calls.size
            off = false
            scope.runCurrent()
            assertEquals("reading the hint asks nothing", count, fake.calls.size)
            at(2_000)
            assertFalse((seen.views.last().second as MachineView.Unreachable).vpnHint)
        }
    }
}
