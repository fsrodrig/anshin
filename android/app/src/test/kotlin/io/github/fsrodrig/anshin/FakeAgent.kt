// currentTime and runCurrent are the virtual clock these tests are about; kotlinx-coroutines-test still marks them experimental.
@file:OptIn(kotlinx.coroutines.ExperimentalCoroutinesApi::class)

package io.github.fsrodrig.anshin

import kotlin.coroutines.cancellation.CancellationException
import kotlin.time.Duration
import kotlinx.coroutines.delay
import kotlinx.coroutines.test.TestCoroutineScheduler
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrl

/** One request the fake answered, or was still answering when its caller went away. Times are virtual ms. */
data class Call(
    val machine: String,
    val after: String?,
    val wait: Duration?,
    val startMs: Long,
    var endMs: Long? = null,
    var cancelled: Boolean = false,
    var result: PulseResult? = null,
)

/**
 * Scripted Agents behind [AgentLink], one per Machine, on the test's virtual clock. Each takes a Sample every 2 s at
 * 1.5 s, 3.5 s, …, so no Sample ties with a request on whole or half seconds, and answers a Cursor as the real
 * recorder does: missing or foreign → full, known → delta, at the newest with `wait` → held until the next Sample or
 * the wait ends. Its bodies go through [decodePulse], the adapter's own mapping. It never evicts, so keep tests well
 * under 30 virtual minutes.
 */
class FakeAgent(private val scheduler: TestCoroutineScheduler) : AgentLink {
    val calls = mutableListOf<Call>()

    private val machines = mutableMapOf<String, Machine>()

    fun url(machine: String): HttpUrl = "http://$machine.example:7247/v1/pulse".toHttpUrl()

    fun machine(name: String): Machine = machines.getOrPut(name) { Machine(name) }

    fun callsTo(name: String) = calls.filter { it.machine == name }

    override suspend fun pulse(endpoint: HttpUrl, after: String?, wait: Duration?): PulseResult {
        val call = Call(endpoint.host.substringBefore('.'), after, wait, scheduler.currentTime)
        calls += call
        try {
            return machine(call.machine).answer(after, wait, call.startMs).also { call.result = it }
        } catch (e: CancellationException) {
            call.cancelled = true
            throw e
        } finally {
            call.endMs = scheduler.currentTime
        }
    }

    private sealed interface Mode {
        data object Answering : Mode
        data object Asleep : Mode
        data object Stopped : Mode
        data object Forbids : Mode
        data class Status(val code: Int) : Mode
    }

    private data class Row(val boottimeMs: Long?, val cpu: Double?)

    inner class Machine(private val name: String) {
        private var mode: Mode = Mode.Answering
        private var schema = PULSE_SCHEMA
        private var frozen = false
        private var ignoresWait = false
        private var malformed = false
        private var inconsistentOnce = false
        private var epoch = 1L
        private val rows = mutableListOf<Row>()
        private var sampledUntilMs = -1L
        private var slept = false

        init {
            // The Agent has been up a while, so its first full Pulse already has a short Window.
            repeat(3) { rows += Row(boottimeMs = BOOT_OFFSET_MS - 2_000L * (3 - it), cpu = 20.0 + it) }
        }

        private fun now() = scheduler.currentTime

        private fun nextSampleAfter(ms: Long): Long = if (ms < 1_500) 1_500 else 1_500 + ((ms - 1_500) / 2_000 + 1) * 2_000

        private fun takeSample(at: Long) {
            if (mode == Mode.Asleep) {
                slept = true
                return
            }
            if (frozen || mode == Mode.Stopped) return
            if (slept) rows += Row(null, null)
            slept = false
            rows += Row(BOOT_OFFSET_MS + at, 20.0 + (at / 2_000) % 10)
        }

        private fun advance() {
            val now = now()
            var t = nextSampleAfter(sampledUntilMs)
            while (t <= now) {
                takeSample(t)
                sampledUntilMs = t
                t = nextSampleAfter(t)
            }
        }

        private fun set(change: () -> Unit) {
            advance()
            change()
        }

        /**
         * The Machine sleeps: a new request fails at the 5 s connect timeout, and one it was holding at the read
         * timeout, 5 s plus its wait after it started.
         */
        fun asleep() = set { mode = Mode.Asleep }

        /** The Agent's port refuses connections at once. */
        fun stopped() = set { mode = Mode.Stopped }

        fun answering() = set { mode = Mode.Answering }

        fun forbids() = set { mode = Mode.Forbids }

        fun status(code: Int) = set { mode = Mode.Status(code) }

        fun speaks(schema: Int) = set { this.schema = schema }

        /** The Agent answers but takes no new Sample. */
        fun frozen(frozen: Boolean = true) = set { this.frozen = frozen }

        /** A new Agent run: a new epoch and a Window of its own. */
        fun restarts() = set {
            epoch++
            rows.clear()
        }

        /** The Agent answers at once even when asked to hold. */
        fun ignoresWait() = set { ignoresWait = true }

        /** Every full Pulse carries a Window without its span, which the phone can't hold. */
        fun sendsMalformedWindows() = set { malformed = true }

        /** The next delta claims one row more than it carries. */
        fun sendsAnInconsistentDelta() = set { inconsistentOnce = true }

        private val newest get() = rows.size

        private fun cursor() = "%016x.%d".format(epoch, newest)

        suspend fun answer(after: String?, wait: Duration?, startMs: Long): PulseResult {
            advance()
            // A Machine already asleep never accepts the connection, so the call fails at the connect timeout.
            refusal(startMs, null)?.let { return it }
            val since = after?.let(::seqOf)
            if (since == null || wait == null || ignoresWait || since != newest) return body(since)
            val until = startMs + wait.inWholeMilliseconds
            while (now() < until) {
                delay(minOf(until, nextSampleAfter(now()), now() + POLL_MS) - now())
                advance()
                refusal(startMs, wait)?.let { return it }
                if (newest != since) return body(since)
            }
            return body(since)
        }

        private suspend fun refusal(startMs: Long, wait: Duration?): PulseResult? = when (val m = mode) {
            Mode.Asleep -> hang(startMs, wait)
            Mode.Stopped -> PulseResult.Unreachable(outsideTailnet = false)
            Mode.Forbids -> PulseResult.Forbidden
            is Mode.Status -> PulseResult.Failed(m.code)
            Mode.Answering -> null
        }

        private suspend fun hang(startMs: Long, wait: Duration?): PulseResult {
            delay(startMs + TIMEOUT_MS + (wait?.inWholeMilliseconds ?: 0) - now())
            return PulseResult.Unreachable(outsideTailnet = false)
        }

        private fun seqOf(cursor: String): Int? {
            val (e, s) = cursor.split('.').takeIf { it.size == 2 } ?: return null
            val seq = s.toIntOrNull() ?: return null
            return seq.takeIf { e.toLongOrNull(16) == epoch && seq in 0..newest }
        }

        private fun body(since: Int?): PulseResult {
            val from = since ?: 0
            val kind = if (since == null) "full" else "delta"
            val window = rows.drop(from)
            var cursor = cursor()
            if (since != null && inconsistentOnce) {
                inconsistentOnce = false
                cursor = "%016x.%d".format(epoch, newest + 1)
            }
            val span = if (malformed && since == null) "" else "\"span_s\":1800,"
            val newestCpu = rows.lastOrNull { it.boottimeMs != null }?.cpu
            val json = """{"schema":$schema,"kind":"$kind","cursor":"$cursor","hostname":"$name","cpu_pct":$newestCpu,
                "verdict":{"level":"crit","reasons":[{"code":"cpu_hot","level":"crit","message":"CPU at 97.0 °C"}]},
                "window":{$span"boottime_ms":${window.map { it.boottimeMs }},"cpu_pct":${window.map { it.cpu }}}}"""
            return decodePulse(json)?.let(PulseResult::Ok) ?: PulseResult.Incompatible
        }
    }

    companion object {
        /** The Machine's boot clock at virtual time zero: well past 30 minutes, so no Window starts before boot. */
        const val BOOT_OFFSET_MS = 3_600_000L

        /** OkHttp's connect and read timeouts in [TailnetHttp]. */
        const val TIMEOUT_MS = 5_000L

        /** How often a held request looks at the Machine, so a mode change reaches it. */
        private const val POLL_MS = 100L
    }
}
