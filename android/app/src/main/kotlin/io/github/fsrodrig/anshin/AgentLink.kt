package io.github.fsrodrig.anshin

import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlin.time.Duration
import kotlinx.coroutines.Dispatchers
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
    /**
     * [after] is the Cursor of the Window the phone holds. [wait] asks the Agent to hold the request until its next
     * Sample, so the call is allowed that much longer than the client's own timeouts.
     */
    suspend fun pulse(endpoint: HttpUrl, after: String? = null, wait: Duration? = null): PulseResult {
        require(wait == null || !wait.isNegative()) { "wait must not be negative" }
        val url = endpoint.newBuilder().apply {
            after?.let { addQueryParameter("after", it) }
            wait?.let { addQueryParameter("wait", it.inWholeSeconds.toString()) }
        }.build()
        // newBuilder shares the socket factory, NO_PROXY and redirect policy, so the destination check still applies.
        val caller = if (wait == null) client else client.newBuilder()
            .readTimeout(client.readTimeoutMillis.extendedBy(wait), TimeUnit.MILLISECONDS)
            .callTimeout(client.callTimeoutMillis.extendedBy(wait), TimeUnit.MILLISECONDS)
            .build()
        return try {
            caller.newCall(Request.Builder().url(url).build()).executeAsync().use { response ->
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
    }
}

// OkHttp's 0 means no timeout, which a hold must not turn into one.
private fun Int.extendedBy(wait: Duration): Long = if (this == 0) 0 else this + wait.inWholeMilliseconds
