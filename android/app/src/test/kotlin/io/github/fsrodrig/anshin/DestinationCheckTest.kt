package io.github.fsrodrig.anshin

import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.ServerSocket
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class DestinationCheckTest {
    // A live listener on loopback, so a refused connect proves the check refused it rather than the network.
    private lateinit var listener: ServerSocket

    @Before
    fun listen() {
        listener = ServerSocket(0, 1, InetAddress.getLoopbackAddress())
    }

    @After
    fun close() {
        listener.close()
    }

    private fun address(literal: String) = InetAddress.getByName(literal)

    @Test
    fun ipv4InsideTheRangeIsAllowedUpToBothEdges() {
        for (literal in listOf("100.64.0.0", "100.64.0.1", "100.100.100.100", "100.127.255.255")) {
            assertTrue(literal, TailnetAddresses.contains(address(literal)))
            requireTailnet(InetSocketAddress(address(literal), AGENT_PORT))
        }
    }

    @Test
    fun ipv4JustOutsideTheRangeIsRefused() {
        for (literal in listOf("100.63.255.255", "100.128.0.0", "127.0.0.1", "192.168.1.10", "10.0.0.1", "0.0.0.0")) {
            assertFalse(literal, TailnetAddresses.contains(address(literal)))
            assertThrows(literal, OutsideTailnetException::class.java) {
                requireTailnet(InetSocketAddress(address(literal), AGENT_PORT))
            }
        }
    }

    @Test
    fun ipv6InsideTheRangeIsAllowedUpToBothEdges() {
        for (literal in listOf("fd7a:115c:a1e0::", "fd7a:115c:a1e0::1", "fd7a:115c:a1e0:ffff:ffff:ffff:ffff:ffff")) {
            assertTrue(literal, TailnetAddresses.contains(address(literal)))
            requireTailnet(InetSocketAddress(address(literal), AGENT_PORT))
        }
    }

    @Test
    fun ipv6OutsideTheRangeIsRefused() {
        for (literal in listOf("fd7a:115c:a1e1::1", "fd7a:115c:a1df:ffff::1", "::1", "fe80::1", "::ffff:192.168.1.10")) {
            assertFalse(literal, TailnetAddresses.contains(address(literal)))
            assertThrows(literal, OutsideTailnetException::class.java) {
                requireTailnet(InetSocketAddress(address(literal), AGENT_PORT))
            }
        }
    }

    @Test
    fun anUnresolvedAddressIsRefusedEvenWhenItsNameLooksLikeATailnetAddress() {
        for (name in listOf("100.64.0.1", "fd7a:115c:a1e0::1", "machine.example.ts.net")) {
            assertThrows(name, OutsideTailnetException::class.java) {
                requireTailnet(InetSocketAddress.createUnresolved(name, AGENT_PORT))
            }
        }
    }

    @Test
    fun theTwoArgumentConnectIsChecked() {
        TailnetSocket().use { socket ->
            assertThrows(OutsideTailnetException::class.java) { socket.connect(listener.localSocketAddress, 1_000) }
            assertFalse(socket.isConnected)
        }
    }

    @Test
    fun theOneArgumentConnectIsChecked() {
        TailnetSocket().use { socket ->
            assertThrows(OutsideTailnetException::class.java) { socket.connect(listener.localSocketAddress) }
            assertFalse(socket.isConnected)
        }
    }

    @Test
    fun theFactoryChecksSocketsItConnectsItself() {
        val loopback = InetAddress.getLoopbackAddress()
        val port = listener.localPort
        assertThrows(OutsideTailnetException::class.java) { TailnetSocketFactory.createSocket(loopback, port) }
        assertThrows(OutsideTailnetException::class.java) { TailnetSocketFactory.createSocket("127.0.0.1", port) }
        assertThrows(OutsideTailnetException::class.java) {
            TailnetSocketFactory.createSocket(loopback, port, loopback, 0)
        }
        assertThrows(OutsideTailnetException::class.java) {
            TailnetSocketFactory.createSocket("127.0.0.1", port, loopback, 0)
        }
    }

    @Test
    fun theListenerIsReachableWithoutTheCheck() {
        java.net.Socket().use { socket ->
            socket.connect(listener.localSocketAddress, 1_000)
            assertTrue(socket.isConnected)
        }
    }
}
