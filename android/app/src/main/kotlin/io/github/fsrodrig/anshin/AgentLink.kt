package io.github.fsrodrig.anshin

import java.io.IOException
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.coroutines.executeAsync

sealed interface PulseResult {
    data class Ok(val pulse: Pulse) : PulseResult

    /** No answer. [outsideTailnet] is set when Anshin itself refused, because the address isn't on the tailnet. */
    data class Unreachable(val outsideTailnet: Boolean) : PulseResult

    data object Forbidden : PulseResult

    data object Incompatible : PulseResult

    /** Any other HTTP answer, such as 503 when the Agent can't reach tailscaled. */
    data class Failed(val httpStatus: Int) : PulseResult
}

/**
 * Asks an Agent for its Pulse. Production passes [TailnetHttp.client]; tests pass a client that can reach a loopback
 * MockWebServer, which the destination check would rightly refuse.
 */
class AgentLink(private val client: OkHttpClient) {
    suspend fun pulse(endpoint: HttpUrl): PulseResult = try {
        client.newCall(Request.Builder().url(endpoint).build()).executeAsync().use { response ->
            when (response.code) {
                200 -> withContext(Dispatchers.IO) { decodePulse(response.body.string()) }
                    ?.let(PulseResult::Ok)
                    ?: PulseResult.Incompatible
                403 -> PulseResult.Forbidden
                else -> PulseResult.Failed(response.code)
            }
        }
    } catch (_: OutsideTailnetException) {
        PulseResult.Unreachable(outsideTailnet = true)
    } catch (_: IOException) {
        PulseResult.Unreachable(outsideTailnet = false)
    }

    /** Polls while collected, and stops the moment collection stops: there is no background work. */
    fun pulses(endpoint: HttpUrl, interval: Duration = 2.seconds): Flow<PulseResult> = flow {
        while (true) {
            emit(pulse(endpoint))
            delay(interval)
        }
    }
}
