package io.github.fsrodrig.anshin

import java.time.Duration
import javax.net.SocketFactory
import kotlinx.coroutines.runBlocking
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.SocketEffect
import okhttp3.Headers.Companion.headersOf
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

class AgentLinkTest {
    private val server = MockWebServer()

    // The production client minus only the destination check, which would refuse this loopback server. The check
    // itself is tested in DestinationCheckTest and ProxyBypassTest; it is never widened for tests.
    private val link = AgentLink(
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

    private fun pulse() = runBlocking { link.pulse(server.url("/v1/pulse")) }

    @Test
    fun aPulseIsOk() {
        server.enqueue(MockResponse(body = """{"schema":1,"kind":"full","hostname":"machine","cpu_pct":7}"""))
        assertEquals(PulseResult.Ok(Pulse(schema = 1, hostname = "machine", cpuPct = 7.0)), pulse())
        assertEquals("/v1/pulse", server.takeRequest().url.encodedPath)
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
        assertEquals(PulseResult.Unreachable(outsideTailnet = false), runBlocking { link.pulse(url) })
    }
}
