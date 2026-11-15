package io.github.fsrodrig.anshin

import javax.net.SocketFactory
import kotlin.time.Duration
import kotlinx.coroutines.flow.take
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test

class FollowTest {
    private val server = MockWebServer()

    // As in AgentLinkTest: the production client minus only the destination check, which refuses loopback.
    private val link = AgentLink(TailnetHttp.newClient().newBuilder().socketFactory(SocketFactory.getDefault()).build())

    @Before
    fun start() {
        server.start()
    }

    @After
    fun stop() {
        server.close()
    }

    private fun follow(answers: Int): List<Followed> =
        runBlocking { follow(link, server.url("/v1/pulse"), interval = Duration.ZERO).take(answers).toList() }

    private val full = """{"schema":1,"kind":"full","cursor":"00000000000000ff.2","cpu_pct":3,
        "window":{"span_s":1800,"boottime_ms":[1000,3000],"cpu_pct":[null,3]}}"""
    private val delta = """{"schema":1,"kind":"delta","cursor":"00000000000000ff.4","cpu_pct":5,
        "window":{"span_s":1800,"boottime_ms":[5000,7000],"cpu_pct":[4,5]}}"""

    @Test
    fun theFirstRequestAsksForTheWholeWindowAndTheNextSendsItsCursor() {
        server.enqueue(MockResponse(body = full))
        server.enqueue(MockResponse(body = delta))
        val answers = follow(2)
        assertNull(server.takeRequest().url.queryParameter("after"))
        val second = server.takeRequest().url
        assertEquals("00000000000000ff.2", second.queryParameter("after"))
        assertNull(second.queryParameter("wait"))

        assertEquals(listOf(1000L, 3000L), answers[0].window!!.window.boottimeMs)
        assertEquals("00000000000000ff.4", answers[1].window!!.cursor)
        assertEquals(listOf(1000L, 3000L, 5000L, 7000L), answers[1].window!!.window.boottimeMs)
        assertEquals(listOf(null, 3.0, 4.0, 5.0), answers[1].window!!.window.cpuPct)
    }

    @Test
    fun aFailedRequestKeepsTheWindowAndItsCursor() {
        server.enqueue(MockResponse(body = full))
        server.enqueue(MockResponse(code = 500))
        server.enqueue(MockResponse(body = delta))
        val answers = follow(3)
        assertEquals(PulseResult.Failed(500), answers[1].result)
        assertEquals(answers[0].window, answers[1].window)
        server.takeRequest()
        assertEquals("00000000000000ff.2", server.takeRequest().url.queryParameter("after"))
        assertEquals("00000000000000ff.2", server.takeRequest().url.queryParameter("after"))
        assertEquals(4, answers[2].window!!.window.rows)
    }

    @Test
    fun anInconsistentDeltaDropsTheWindowSoTheNextRequestAsksForAllOfIt() {
        server.enqueue(MockResponse(body = full))
        server.enqueue(MockResponse(body = delta.replace("00000000000000ff.4", "0000000000000001.4")))
        server.enqueue(MockResponse(body = full))
        val answers = follow(3)
        assertNull(answers[1].window)
        server.takeRequest()
        server.takeRequest()
        assertNull(server.takeRequest().url.queryParameter("after"))
        assertEquals(2, answers[2].window!!.window.rows)
    }
}
