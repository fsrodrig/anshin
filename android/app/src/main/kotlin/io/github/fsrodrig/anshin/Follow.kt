package io.github.fsrodrig.anshin

import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import okhttp3.HttpUrl

/** The latest answer and the Window the phone holds after it. A failed request keeps the Window it had. */
data class Followed(val result: PulseResult, val window: HeldWindow?)

/**
 * Polls while collected, and stops the moment collection stops: there is no background work. Every request after the
 * first full Pulse sends the held Cursor, so the Agent answers with only the newer Samples. #10's Watcher replaces
 * this loop.
 */
fun follow(link: AgentLink, endpoint: HttpUrl, interval: Duration = 2.seconds): Flow<Followed> = flow {
    var held: HeldWindow? = null
    while (true) {
        val result = link.pulse(endpoint, after = held?.cursor)
        if (result is PulseResult.Ok) held = held.merge(result.pulse)
        emit(Followed(result, held))
        delay(interval)
    }
}
