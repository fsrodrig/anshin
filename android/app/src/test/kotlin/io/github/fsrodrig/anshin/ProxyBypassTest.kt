package io.github.fsrodrig.anshin

import java.io.IOException
import java.net.ProxySelector
import java.net.SocketAddress
import java.net.URI
import java.time.Duration
import kotlinx.coroutines.runBlocking
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Before
import org.junit.Test

/** A system proxy must not be able to carry a request past the destination check. */
class ProxyBypassTest {
    private val proxy = MockWebServer()
    private val previousSelector = ProxySelector.getDefault()
    private val proxyProperties = listOf("http.proxyHost", "http.proxyPort")
    private val previousProperties = proxyProperties.associateWith { System.getProperty(it) }

    @Before
    fun configureSystemProxy() {
        proxy.start()
        ProxySelector.setDefault(
            object : ProxySelector() {
                override fun select(uri: URI?) = listOf(proxy.proxyAddress)

                override fun connectFailed(uri: URI?, address: SocketAddress?, e: IOException?) = Unit
            },
        )
        System.setProperty("http.proxyHost", proxy.hostName)
        System.setProperty("http.proxyPort", proxy.port.toString())
    }

    @After
    fun restore() {
        ProxySelector.setDefault(previousSelector)
        previousProperties.forEach { (key, value) ->
            if (value == null) System.clearProperty(key) else System.setProperty(key, value)
        }
        proxy.close()
    }

    @Test
    fun theProxyIsLiveForAnOrdinaryClient() {
        proxy.enqueue(MockResponse(code = 200, body = "from the proxy"))
        OkHttpClient().newCall(Request.Builder().url("http://192.168.1.10:7247/v1/pulse").build()).execute().use {
            assertEquals("from the proxy", it.body.string())
        }
        assertEquals(1, proxy.requestCount)
    }

    @Test
    fun anOutsideAddressIsRefusedAndNeverReachesTheProxy() {
        val call = TailnetHttp.newClient().newCall(Request.Builder().url("http://192.168.1.10:7247/v1/pulse").build())
        assertThrows(IOException::class.java) { call.execute() }
        assertEquals(0, proxy.requestCount)
    }

    @Test
    fun anOutsideAddressIsUnreachableThroughTheAgentLink() = runBlocking {
        val result = AgentLink(TailnetHttp.newClient()).pulse("http://127.0.0.1:${proxy.port}/v1/pulse".toHttpUrl())
        assertEquals(PulseResult.Unreachable(outsideTailnet = true), result)
        assertEquals(0, proxy.requestCount)
    }

    @Test
    fun aTailnetAddressGoesDirectlyNotThroughTheProxy() {
        // Whether anything answers at the fixture address doesn't matter; only that the proxy never saw the request.
        val client = TailnetHttp.newClient().newBuilder().callTimeout(Duration.ofMillis(500)).build()
        runCatching { client.newCall(Request.Builder().url("http://100.64.0.1:7247/v1/pulse").build()).execute().close() }
        assertEquals(0, proxy.requestCount)
    }
}
