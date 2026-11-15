package io.github.fsrodrig.anshin

import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import javax.net.SocketFactory
import kotlin.time.TimeSource
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.Call
import okhttp3.EventListener
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** The Watcher over the real adapter, on the wire: what it asks, and that leaving the screen hangs up. */
class WatcherHttpTest {
    private val server = MockWebServer()
    private val canceled = CountDownLatch(1)

    // As in AgentLinkTest: the production client minus only the destination check, which refuses loopback.
    private val link = OkHttpAgentLink(
        TailnetHttp.newClient().newBuilder()
            .socketFactory(SocketFactory.getDefault())
            .eventListener(object : EventListener() {
                override fun canceled(call: Call) = canceled.countDown()
            })
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

    @Test
    fun theSecondRequestLongPollsWithTheCursorAndCancellingCollectionHangsUpTheHeldRequest() = runBlocking {
        server.enqueue(MockResponse(body = """{"schema":1,"kind":"full","cursor":"00000000000000ff.2","cpu_pct":3,
            "window":{"span_s":1800,"boottime_ms":[1000,3000],"cpu_pct":[null,3]}}"""))
        // An Agent holding the long-poll: no answer for far longer than the test waits.
        server.enqueue(MockResponse.Builder().body("""{"schema":1}""").headersDelay(30, TimeUnit.SECONDS).build())

        val views = mutableListOf<MachineView>()
        val watcher = Watcher(link, TimeSource.Monotonic)
        val collector = CoroutineScope(Dispatchers.Default).launch {
            watcher.watch(server.url("/v1/pulse")).collect { synchronized(views) { views += it } }
        }
        val first = server.takeRequest(5, TimeUnit.SECONDS)!!.url
        assertNull(first.queryParameter("after"))
        assertNull(first.queryParameter("wait"))
        val held = server.takeRequest(5, TimeUnit.SECONDS)!!.url
        assertEquals("00000000000000ff.2", held.queryParameter("after"))
        assertEquals("10", held.queryParameter("wait"))
        assertTrue(synchronized(views) { views.last() is MachineView.Live })

        collector.cancelAndJoin()
        assertTrue("OkHttp canceled the held call", canceled.await(5, TimeUnit.SECONDS))
        assertNull("nothing more is asked", server.takeRequest(1500, TimeUnit.MILLISECONDS))
        assertEquals(2, server.requestCount)
    }
}
