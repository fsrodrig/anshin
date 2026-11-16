package io.github.fsrodrig.anshin

import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference
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
import okhttp3.ResponseBody
import okio.Buffer
import okio.ForwardingSource
import okio.buffer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class WatcherHttpTest {
    private val server = MockWebServer()
    private val canceled = CountDownLatch(1)
    private val bodyStarted = CountDownLatch(1)
    private val bodyCall = AtomicReference<Call>()
    private val bodyCanceled = CountDownLatch(1)
    private val canceledPorts = ConcurrentHashMap.newKeySet<Int>()

    // As in AgentLinkTest: the production client minus only the destination check, which refuses loopback.
    private val link = OkHttpAgentLink(
        TailnetHttp.newClient().newBuilder()
            .socketFactory(SocketFactory.getDefault())
            .addInterceptor { chain ->
                val response = chain.proceed(chain.request())
                if (chain.request().url.queryParameter("after") == null) response else {
                    val body = response.body
                    val source = object : ForwardingSource(body.source()) {
                        override fun read(sink: Buffer, byteCount: Long): Long {
                            bodyCall.set(chain.call())
                            bodyStarted.countDown()
                            return super.read(sink, byteCount)
                        }
                    }.buffer()
                    response.newBuilder().body(object : ResponseBody() {
                        override fun contentType() = body.contentType()
                        override fun contentLength() = body.contentLength()
                        override fun source() = source
                    }).build()
                }
            }
            .eventListener(object : EventListener() {
                override fun canceled(call: Call) {
                    canceledPorts += call.request().url.port
                    canceled.countDown()
                    if (call === bodyCall.get()) bodyCanceled.countDown()
                }
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
    fun cancellingCollectionDuringTheHeldResponseBodyReadCancelsThatCallPromptly() = runBlocking {
        server.enqueue(MockResponse(body = """{"schema":1,"kind":"full","cursor":"00000000000000ff.2",
            "window":{"span_s":1800,"boottime_ms":[1000,3000],"cpu_pct":[null,3]}}"""))
        server.enqueue(MockResponse.Builder().body("""{"schema":1,"kind":"delta"}""")
            .bodyDelay(2, TimeUnit.SECONDS).build())
        val collector = CoroutineScope(Dispatchers.Default).launch {
            Watcher(link, TimeSource.Monotonic).watch(server.url("/v1/pulse")).collect {}
        }
        try {
            assertTrue("the held call is consuming its body", bodyStarted.await(5, TimeUnit.SECONDS))
            collector.cancel()
            assertTrue("the same call is canceled during body consumption", bodyCanceled.await(500, TimeUnit.MILLISECONDS))
            assertTrue(bodyCall.get().isCanceled())
        } finally {
            collector.cancelAndJoin()
        }
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

    @Test
    fun aMachineThatNeverAnswersNeverDelaysAnotherOverHttp() = runBlocking {
        MockWebServer().use { silent ->
            silent.start()
            silent.enqueue(MockResponse.Builder().body("""{"schema":1}""").headersDelay(30, TimeUnit.SECONDS).build())
            server.enqueue(MockResponse(body = """{"schema":1,"kind":"full","cursor":"00000000000000ff.2","cpu_pct":3,
                "window":{"span_s":1800,"boottime_ms":[1000,3000],"cpu_pct":[null,3]}}"""))
            server.enqueue(MockResponse.Builder().body("""{"schema":1}""").headersDelay(30, TimeUnit.SECONDS).build())

            val watcher = Watcher(link, TimeSource.Monotonic)
            val a = AtomicReference<MachineView>()
            val b = AtomicReference<MachineView>()
            val scope = CoroutineScope(Dispatchers.Default)
            val collectors = listOf(
                scope.launch { watcher.watch(silent.url("/v1/pulse")).collect { a.set(it) } },
                scope.launch { watcher.watch(server.url("/v1/pulse")).collect { b.set(it) } },
            )
            try {
                assertNotNull("a is asked first", silent.takeRequest(5, TimeUnit.SECONDS))
                val started = System.nanoTime()
                while (b.get() !is MachineView.Live && System.nanoTime() - started < 2_000_000_000L) Thread.sleep(10)
                assertTrue("b is Live within 2 s while a has not answered", b.get() is MachineView.Live)
                assertEquals(MachineView.Connecting, a.get())
            } finally {
                collectors.forEach { it.cancelAndJoin() }
            }
            val deadline = System.nanoTime() + 5_000_000_000L
            while (silent.port !in canceledPorts && System.nanoTime() < deadline) Thread.sleep(10)
            assertTrue("OkHttp canceled the call a never answered", silent.port in canceledPorts)
        }
    }
}
