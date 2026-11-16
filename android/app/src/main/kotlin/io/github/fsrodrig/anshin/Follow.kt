package io.github.fsrodrig.anshin

import androidx.compose.runtime.Immutable
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds
import kotlin.time.TimeMark
import kotlin.time.TimeSource
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import okhttp3.HttpUrl

/** The last Pulse the phone received and when it arrived, on the phone's monotonic clock. */
@Immutable
data class LastPulse(val pulse: Pulse, val at: TimeMark)

/**
 * The latest answer, the Window the phone holds after it and the last Pulse it received. A failed request keeps the
 * Window and the last Pulse it had.
 */
data class Followed(val result: PulseResult, val window: HeldWindow?, val last: LastPulse? = null)

/**
 * Polls while collected, and stops the moment collection stops: there is no background work. Every request after the
 * first full Pulse sends the held Cursor, so the Agent answers with only the newer Samples. #10's Watcher replaces
 * this loop.
 */
fun follow(
    link: AgentLink,
    endpoint: HttpUrl,
    interval: Duration = 2.seconds,
    clock: TimeSource = TimeSource.Monotonic,
): Flow<Followed> = flow {
    var held: HeldWindow? = null
    var last: LastPulse? = null
    while (true) {
        val result = link.pulse(endpoint, after = held?.cursor)
        if (result is PulseResult.Ok) {
            last = LastPulse(result.pulse, clock.markNow())
            held = held.merge(result.pulse)
        }
        emit(Followed(result, held, last))
        delay(interval)
    }
}

/**
 * Interim, until #10's Watcher: Live on an Ok answer, Unreachable while a last Pulse exists, and `null` for every
 * state that withholds data. An Agent that answered (403, an unreadable Pulse, 503) isn't Unreachable, so its old
 * values aren't shown.
 */
internal fun Followed?.detail(machine: String): MachineDetailState? {
    val last = this?.last ?: return null
    val connection = when (val answer = result) {
        is PulseResult.Ok -> Connection.Live
        is PulseResult.Unreachable -> Connection.Unreachable(answer.outsideTailnet)
        else -> return null
    }
    return MachineDetailState(machine, connection, last, window)
}
