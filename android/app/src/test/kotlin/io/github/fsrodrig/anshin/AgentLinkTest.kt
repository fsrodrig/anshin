package io.github.fsrodrig.anshin

import java.time.Duration
import java.util.concurrent.TimeUnit
import javax.net.SocketFactory
import kotlin.time.Duration.Companion.seconds
import kotlinx.coroutines.runBlocking
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.SocketEffect
import okhttp3.Headers.Companion.headersOf
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class AgentLinkTest {
    private val server = MockWebServer()

    // The production client minus only the destination check, which would refuse this loopback server. The check
    // itself is tested in DestinationCheckTest and ProxyBypassTest; it is never widened for tests.
    private val link = OkHttpAgentLink(
        TailnetHttp.newClient().newBuilder()
            .socketFactory(SocketFactory.getDefault())
            .readTimeout(Duration.ofMillis(300))
            .build(),
    )

    @Before
    fun start() {
        server.start()
    }

    @After
    fun stop() {
        server.close()
    }

    private fun pulse() = runBlocking { link.pulse(server.url("/v1/pulse"), after = null, wait = null) }

    @Test
    fun aPulseIsOk() {
        server.enqueue(MockResponse(body = """{"schema":1,"kind":"full","hostname":"machine","cpu_pct":7}"""))
        assertEquals(
            PulseResult.Ok(Pulse(schema = 1, kind = PulseKind.Full, hostname = "machine", cpuPct = 7.0)),
            pulse(),
        )
        assertEquals("/v1/pulse", server.takeRequest().url.encodedPath)
    }

    @Test
    fun aRequestCarriesNoCredentialAndNothingAboutThePhone() {
        server.enqueue(MockResponse(body = """{"schema":1,"kind":"full"}"""))
        pulse()
        val headers = server.takeRequest().headers
        assertEquals(
            setOf("host", "connection", "accept-encoding", "user-agent"),
            headers.names().map { it.lowercase() }.toSet(),
        )
        // OkHttp's default names the library and its version, not the phone or the app.
        assertTrue(headers["User-Agent"].orEmpty(), headers["User-Agent"].orEmpty().matches(Regex("okhttp/[0-9.]+")))
    }

    @Test
    fun http403IsForbidden() {
        server.enqueue(MockResponse(code = 403))
        assertEquals(PulseResult.Forbidden, pulse())
    }

    @Test
    fun http503IsKeptForTheScreenToExplain() {
        server.enqueue(MockResponse(code = 503))
        assertEquals(PulseResult.Failed(503), pulse())
    }

    @Test
    fun anUnknownSchemaIsIncompatible() {
        server.enqueue(MockResponse(body = """{"schema":2,"kind":"full"}"""))
        assertEquals(PulseResult.Incompatible, pulse())
    }

    @Test
    fun somethingOtherThanAPulseIsIncompatible() {
        server.enqueue(MockResponse(body = "<html>hello</html>"))
        assertEquals(PulseResult.Incompatible, pulse())
    }

    @Test
    fun aRedirectIsNotFollowed() {
        server.enqueue(MockResponse(code = 302, headers = headersOf("Location", "http://192.168.1.10/v1/pulse")))
        assertEquals(PulseResult.Failed(302), pulse())
        assertEquals(1, server.requestCount)
    }

    @Test
    fun noAnswerInTimeIsUnreachable() {
        server.enqueue(MockResponse.Builder().onResponseStart(SocketEffect.Stall).build())
        assertEquals(PulseResult.Unreachable(outsideTailnet = false), pulse())
    }

    @Test
    fun aRefusedConnectionIsUnreachable() {
        val url = server.url("/v1/pulse")
        server.close()
        assertEquals(PulseResult.Unreachable(outsideTailnet = false), runBlocking { link.pulse(url, after = null, wait = null) })
    }

    @Test
    fun theCursorAndWaitAreOnTheRequest() {
        server.enqueue(MockResponse(body = """{"schema":1,"kind":"delta"}"""))
        runBlocking { link.pulse(server.url("/v1/pulse"), after = "00000000000000ff.3", wait = 1.seconds) }
        val url = server.takeRequest().url
        assertEquals("00000000000000ff.3", url.queryParameter("after"))
        assertEquals("1", url.queryParameter("wait"))
        assertEquals(setOf("after", "wait"), url.queryParameterNames)
    }

    @Test
    fun noCursorAndNoWaitAddNothing() {
        server.enqueue(MockResponse(body = """{"schema":1,"kind":"full"}"""))
        pulse()
        assertEquals(emptySet<String>(), server.takeRequest().url.queryParameterNames)
    }

    // The base read timeout is 300 ms here; a held request is allowed its wait on top.
    @Test
    fun aHeldRequestIsGivenItsWait() {
        server.enqueue(
            MockResponse.Builder().body("""{"schema":1,"kind":"delta"}""").headersDelay(700, TimeUnit.MILLISECONDS).build(),
        )
        assertEquals(
            PulseResult.Ok(Pulse(schema = 1, kind = PulseKind.Delta)),
            runBlocking { link.pulse(server.url("/v1/pulse"), after = "00000000000000ff.3", wait = 1.seconds) },
        )
    }

    @Test
    fun withoutAWaitTheSameDelayIsUnreachable() {
        server.enqueue(
            MockResponse.Builder().body("""{"schema":1,"kind":"delta"}""").headersDelay(700, TimeUnit.MILLISECONDS).build(),
        )
        assertEquals(PulseResult.Unreachable(outsideTailnet = false), pulse())
    }
}
