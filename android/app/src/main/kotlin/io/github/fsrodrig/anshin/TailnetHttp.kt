package io.github.fsrodrig.anshin

import java.io.IOException
import java.net.Inet4Address
import java.net.Inet6Address
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.Proxy
import java.net.Socket
import java.net.SocketAddress
import javax.net.SocketFactory
import kotlin.time.Duration.Companion.seconds
import kotlin.time.toJavaDuration
import okhttp3.OkHttpClient

/** Tailscale's address ranges: `100.64.0.0/10` and `fd7a:115c:a1e0::/48`. */
object TailnetAddresses {
    private val ipv6Prefix = byteArrayOf(0xfd.toByte(), 0x7a, 0x11, 0x5c, 0xa1.toByte(), 0xe0.toByte())

    fun contains(address: InetAddress): Boolean {
        val bytes = address.address
        return when (address) {
            is Inet4Address -> bytes[0] == 100.toByte() && (bytes[1].toInt() and 0xc0) == 0x40
            is Inet6Address -> bytes.copyOfRange(0, ipv6Prefix.size).contentEquals(ipv6Prefix)
            else -> false
        }
    }
}

class OutsideTailnetException(endpoint: SocketAddress?) :
    IOException("Refusing to connect to $endpoint: it is outside the tailnet ranges")

/**
 * Throws unless [endpoint] is a resolved address inside the tailnet ranges. An unresolved address counts as outside,
 * because connecting to it would hand name resolution to whatever the socket's caller chose.
 */
fun requireTailnet(endpoint: SocketAddress?) {
    val address = (endpoint as? InetSocketAddress)?.takeUnless { it.isUnresolved }?.address
    if (address == null || !TailnetAddresses.contains(address)) throw OutsideTailnetException(endpoint)
}

/** A socket that checks its destination at the last moment before connecting, whoever asked for the connection. */
internal class TailnetSocket : Socket() {
    override fun connect(endpoint: SocketAddress?) = connect(endpoint, 0)

    override fun connect(endpoint: SocketAddress?, timeout: Int) {
        requireTailnet(endpoint)
        super.connect(endpoint, timeout)
    }
}

internal object TailnetSocketFactory : SocketFactory() {
    override fun createSocket(): Socket = TailnetSocket()

    override fun createSocket(host: String?, port: Int): Socket = connected(InetSocketAddress(host, port))

    override fun createSocket(host: String?, port: Int, localHost: InetAddress?, localPort: Int): Socket =
        connected(InetSocketAddress(host, port), InetSocketAddress(localHost, localPort))

    override fun createSocket(host: InetAddress?, port: Int): Socket = connected(InetSocketAddress(host, port))

    override fun createSocket(address: InetAddress?, port: Int, localAddress: InetAddress?, localPort: Int): Socket =
        connected(InetSocketAddress(address, port), InetSocketAddress(localAddress, localPort))

    private fun connected(endpoint: SocketAddress, local: SocketAddress? = null): Socket {
        val socket = TailnetSocket()
        try {
            if (local != null) socket.bind(local)
            socket.connect(endpoint)
        } catch (e: IOException) {
            socket.close()
            throw e
        }
        return socket
    }
}

/**
 * The app's only HTTP client, and so its only way to reach an address. The destination check lives in the socket,
 * so it covers MagicDNS names and IP literals alike (OkHttp skips `Dns` for literals); `NO_PROXY` keeps a
 * configured system proxy from carrying a request somewhere the check never sees.
 */
object TailnetHttp {
    val client: OkHttpClient by lazy { newClient() }

    internal fun newClient(): OkHttpClient = OkHttpClient.Builder()
        .proxy(Proxy.NO_PROXY)
        .followRedirects(false)
        .followSslRedirects(false)
        .socketFactory(TailnetSocketFactory)
        .connectTimeout(5.seconds.toJavaDuration())
        .readTimeout(5.seconds.toJavaDuration())
        .callTimeout(10.seconds.toJavaDuration())
        .build()
}
