package io.github.fsrodrig.anshin

import android.os.SystemClock
import kotlin.math.pow
import kotlin.random.Random
import kotlin.time.AbstractLongTimeSource
import kotlin.time.ComparableTimeMark
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds
import kotlin.time.DurationUnit
import kotlin.time.TimeSource
import kotlinx.coroutines.Deferred
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.FlowCollector
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.withTimeoutOrNull
import okhttp3.HttpUrl

/** A Pulse with a Sample the phone did not have, the Window held after it, and when it arrived on the Watcher's clock. */
data class Received(val pulse: Pulse, val window: HeldWindow?, val at: ComparableTimeMark)

/** What the phone shows of one Machine. */
sealed interface MachineView {
    /** No answer yet since the Owner asked to watch this Machine. */
    data object Connecting : MachineView

    /** The newest Sample arrived less than [STALE_AFTER] ago. */
    data class Live(val received: Received) : MachineView

    /** The Agent may still answer, but its newest Sample is too old to trust as current. */
    data class Stale(val last: Received) : MachineView {
        val since: ComparableTimeMark get() = last.at
    }

    /**
     * The latest request got no Pulse and the data is not fresh. [last] is what the phone still holds, never current.
     * [vpnHint] only suggests that a VPN is off; it never changes what the Watcher asks. [outsideTailnet] means Anshin
     * itself refused the address; [httpStatus] is set when the Agent answered with something other than a Pulse.
     */
    data class Unreachable(
        val last: Received?,
        val retryAt: ComparableTimeMark,
        val vpnHint: Boolean,
        val outsideTailnet: Boolean = false,
        val httpStatus: Int? = null,
    ) : MachineView

    data object Forbidden : MachineView

    data object Incompatible : MachineView
}

/** Every request that carries a Cursor asks the Agent to hold it this long for the next Sample. */
internal val WAIT = 10.seconds

/** Five missed Samples at the Agent's 2 s cadence: enough to ride out a slow relay or a quick retry without flapping. */
internal val STALE_AFTER = 10.seconds

/** The least time between the starts of two requests, so an Agent that ignores `wait` is never asked in a tight loop. */
internal val LONG_POLL_SPACING = 1.seconds
internal val POLL_SPACING = 2.seconds

internal val BACKOFF_BASE = 1.seconds
internal val BACKOFF_CAP = 16.seconds

/**
 * A 403 or an unreadable Pulse is an answer that changes only when the Owner edits the tailnet policy or upgrades
 * something, so it is asked again about once a minute rather than backed off as a failure.
 */
internal val REFUSED_RETRY = 60.seconds

/** The wait after the [failures]-th failure in a row: doubling from [BACKOFF_BASE] up to [BACKOFF_CAP], jittered. */
internal fun backoff(failures: Int, u: Double): Duration {
    val step = BACKOFF_BASE * 2.0.pow(failures.coerceIn(1, 31) - 1)
    return jittered(minOf(step, BACKOFF_CAP), u)
}

/**
 * Equal jitter: a delay in (step/2, step] for `u` in [0, 1). It never exceeds the step, so [MachineView.Unreachable]'s
 * retry time stays honest, and two Machines that fail together drift apart.
 */
internal fun jittered(step: Duration, u: Double): Duration = step * (1 - u / 2)

/**
 * CLOCK_BOOTTIME, which keeps counting while the phone sleeps. `System.nanoTime` stops in deep sleep, so a phone
 * locked for an hour would show its Stale data as minutes old.
 */
object ElapsedRealtime : AbstractLongTimeSource(DurationUnit.NANOSECONDS) {
    override fun read(): Long = SystemClock.elapsedRealtimeNanos()
}

/**
 * Follows Machines. [clock] stamps every Pulse and is the clock the UI must age them by. [random] only jitters
 * retries. [vpnLooksOff] is advisory: it is read when a request fails and shown, never used to decide anything.
 */
class Watcher(
    private val link: AgentLink,
    val clock: TimeSource.WithComparableMarks,
    private val random: Random = Random.Default,
    private val vpnLooksOff: () -> Boolean = { false },
) {
    /**
     * Follows one Machine while collected and asks nothing while not. Collect it once at a time: collecting this same
     * Flow again resumes with the Window and Cursor it held, and starts from what its data's age says, never from an
     * old Live.
     */
    fun watch(endpoint: HttpUrl): Flow<MachineView> {
        val memory = Memory()
        return flow { Follow(endpoint, memory, this).run() }
    }

    private sealed interface Outcome {
        data object None : Outcome
        data object Answered : Outcome
        data object Forbidden : Outcome
        data object Incompatible : Outcome
        data class Failed(val retryAt: ComparableTimeMark, val vpnHint: Boolean, val outsideTailnet: Boolean, val httpStatus: Int?) : Outcome
    }

    private class Memory {
        var held: HeldWindow? = null
        var last: Received? = null
        var outcome: Outcome = Outcome.None
        var failures = 0
    }

    private inner class Follow(
        private val endpoint: HttpUrl,
        private val memory: Memory,
        private val out: FlowCollector<MachineView>,
    ) {
        private var shown: MachineView? = null

        suspend fun run() {
            // Coming back to the screen asks at once: a backoff or a refused retry from before it was hidden is moot.
            memory.failures = 0
            show()
            coroutineScope {
                while (true) {
                    val start = clock.markNow()
                    val after = memory.held?.cursor
                    val call = async { link.pulse(endpoint, after, wait = if (after == null) null else WAIT) }
                    val next = handle(awaitShowingAge(call), start)
                    show()
                    sleepUntil(next)
                }
            }
        }

        private fun view(): MachineView {
            val outcome = memory.outcome
            if (outcome == Outcome.Forbidden) return MachineView.Forbidden
            if (outcome == Outcome.Incompatible) return MachineView.Incompatible
            val last = memory.last
            val failed = outcome as? Outcome.Failed
            fun unreachable(f: Outcome.Failed) = MachineView.Unreachable(last, f.retryAt, f.vpnHint, f.outsideTailnet, f.httpStatus)
            return when {
                last == null -> failed?.let(::unreachable) ?: MachineView.Connecting
                // A failure while the data is fresh is retried quietly, so an Agent restart never flaps the view.
                clock.markNow() - last.at < STALE_AFTER -> MachineView.Live(last)
                failed != null -> unreachable(failed)
                else -> MachineView.Stale(last)
            }
        }

        private suspend fun show() {
            val view = view()
            if (view != shown) {
                shown = view
                out.emit(view)
            }
        }

        /** How long until the data goes stale, or null when that is not ahead. */
        private fun untilStale(): Duration? {
            val last = memory.last ?: return null
            return (last.at + STALE_AFTER - clock.markNow()).takeIf { it.isPositive() }
        }

        private suspend fun awaitShowingAge(call: Deferred<PulseResult>): PulseResult {
            while (true) {
                show()
                val stale = untilStale() ?: return call.await()
                withTimeoutOrNull(stale) { call.await() }?.let { return it }
                show()
            }
        }

        private suspend fun sleepUntil(next: ComparableTimeMark) {
            while (true) {
                val remaining = next - clock.markNow()
                if (!remaining.isPositive()) return
                val stale = untilStale()
                if (stale == null || stale >= remaining) {
                    delay(remaining)
                    return
                }
                delay(stale)
                show()
            }
        }

        /** Updates the memory with one answer and returns when the next request may start. */
        private fun handle(result: PulseResult, start: ComparableTimeMark): ComparableTimeMark {
            val now = clock.markNow()
            return when (result) {
                is PulseResult.Ok -> {
                    memory.held = memory.held.merge(result.pulse)
                    if (result.pulse.carriesANewSample()) memory.last = Received(result.pulse, memory.held, now)
                    memory.outcome = Outcome.Answered
                    memory.failures = 0
                    start + spacing()
                }
                PulseResult.Forbidden -> refused(Outcome.Forbidden, now)
                PulseResult.Incompatible -> {
                    memory.held = null
                    refused(Outcome.Incompatible, now)
                }
                is PulseResult.Unreachable -> failed(now, start, result.outsideTailnet, null)
                is PulseResult.Failed -> failed(now, start, false, result.httpStatus)
            }
        }

        private fun spacing() = if (memory.held == null) POLL_SPACING else LONG_POLL_SPACING

        private fun refused(outcome: Outcome, now: ComparableTimeMark): ComparableTimeMark {
            memory.outcome = outcome
            memory.failures = 0
            return now + jittered(REFUSED_RETRY, random.nextDouble())
        }

        private fun failed(now: ComparableTimeMark, start: ComparableTimeMark, outsideTailnet: Boolean, httpStatus: Int?): ComparableTimeMark {
            memory.failures++
            val retryAt = maxOf(now + backoff(memory.failures, random.nextDouble()), start + spacing())
            memory.outcome = Outcome.Failed(retryAt, vpnLooksOff(), outsideTailnet, httpStatus)
            return retryAt
        }
    }
}

/**
 * A zero-row delta proves the Agent answers but carries no newer Sample, so it does not make the data current. A
 * kind this app doesn't know is counted as new, since it can't be told apart.
 */
private fun Pulse.carriesANewSample(): Boolean = kind != PulseKind.Delta || (window?.rows ?: 0) > 0
