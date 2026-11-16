package io.github.fsrodrig.anshin

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class MachineAddressTest {
    private fun host(input: String): String? = parseAddress(input, "").address?.host

    private fun problem(input: String): AddressProblem? = parseAddress(input, "").addressProblem

    @Test
    fun acceptsMagicDnsNamesAndTailnetLiteralsInTheirCanonicalForm() {
        val accepted = mapOf(
            "laptop" to "laptop",
            " laptop " to "laptop",
            "Laptop" to "laptop",
            "laptop.example.ts.net" to "laptop.example.ts.net",
            "laptop.example.ts.net." to "laptop.example.ts.net",
            "100.64.0.0" to "100.64.0.0",
            "100.64.0.2" to "100.64.0.2",
            "100.127.255.255" to "100.127.255.255",
            "100.100.100.100" to "100.100.100.100",
            "fd7a:115c:a1e0::" to "fd7a:115c:a1e0::",
            "fd7a:115c:a1e0::1" to "fd7a:115c:a1e0::1",
            "[fd7a:115c:a1e0::1]" to "fd7a:115c:a1e0::1",
            "FD7A:115C:A1E0::1" to "fd7a:115c:a1e0::1",
            "fd7a:115c:a1e0::0001" to "fd7a:115c:a1e0::1",
            "fd7a:115c:a1e0:ffff:ffff:ffff:ffff:ffff" to "fd7a:115c:a1e0:ffff:ffff:ffff:ffff:ffff",
            "::ffff:100.64.0.2" to "100.64.0.2",
        )
        for ((input, canonical) in accepted) {
            val parsed = parseAddress(input, "")
            assertEquals(input, canonical, parsed.address?.host)
            assertEquals(input, AGENT_PORT, parsed.address?.port)
            assertNull(input, parsed.addressProblem)
        }
    }

    @Test
    fun rejectsLiteralsOutsideTheTailnetRanges() {
        val outside = mapOf(
            "192.0.2.1" to "192.0.2.1",
            "100.63.255.255" to "100.63.255.255",
            "100.128.0.0" to "100.128.0.0",
            "127.0.0.1" to "127.0.0.1",
            "10.0.0.1" to "10.0.0.1",
            "0.0.0.0" to "0.0.0.0",
            "::1" to "::1",
            "fe80::1" to "fe80::1",
            "fd7a:115c:a1e1::1" to "fd7a:115c:a1e1::1",
            "fd7a:115c:a1df:ffff::1" to "fd7a:115c:a1df:ffff::1",
            "::ffff:192.0.2.1" to "192.0.2.1",
        )
        for ((input, literal) in outside) {
            assertEquals(input, AddressProblem.OutsideTailnet(literal), problem(input))
            assertNull(input, host(input))
        }
    }

    @Test
    fun rejectsWhatIsNotAMachineAddress() {
        val label64 = "a".repeat(64)
        val name254 = listOf("a".repeat(63), "a".repeat(63), "a".repeat(63), "a".repeat(62)).joinToString(".")
        val malformed = listOf(
            "-laptop", "laptop-", "lap_top", "a..b", ".laptop", label64, name254, "läptop",
            "laptop.123", "100.64.0", "100.064.0.1", "100.64.0.256", "1234", "fd7a:zz::1", "[fd7a:115c:a1e0::1", "[::1]x",
            "http://laptop", "laptop/v1/pulse", "user@laptop", "two words", "laptop:port", "fe80::1%wlan0",
        )
        for (input in malformed) {
            assertEquals(input, AddressProblem.Malformed, problem(input))
            assertNull(input, host(input))
        }
        for (input in listOf("laptop:7247", "100.64.0.2:7247", "[fd7a:115c:a1e0::1]:7247")) {
            assertEquals(input, AddressProblem.HasPort, problem(input))
        }
        for (input in listOf("", "  ")) assertEquals(AddressProblem.Blank, problem(input))
    }

    @Test
    fun aLongestValidNameIsAccepted() {
        val name253 = listOf("a".repeat(63), "a".repeat(63), "a".repeat(63), "a".repeat(61)).joinToString(".")
        assertEquals(name253, host(name253))
    }

    @Test
    fun aBlankPortIsTheAgentsAndAnyOtherMustBeOneTo65535() {
        assertEquals(AGENT_PORT, parseAddress("laptop", "").address?.port)
        assertEquals(AGENT_PORT, parseAddress("laptop", "  ").address?.port)
        assertEquals(9000, parseAddress("laptop", " 9000 ").address?.port)
        assertEquals(AGENT_PORT, parseAddress("laptop", "07247").address?.port)
        assertEquals(1, parseAddress("laptop", "1").address?.port)
        assertEquals(65535, parseAddress("laptop", "65535").address?.port)
        for (port in listOf("0", "65536", "-1", "+1", "7a", "72 47", "99999999999")) {
            val parsed = parseAddress("laptop", port)
            assertEquals(port, PortProblem, parsed.portProblem)
            assertNull(port, parsed.address)
            assertNull(port, parsed.addressProblem)
        }
    }

    @Test
    fun bothFieldsAreReportedTogether() {
        val parsed = parseAddress("192.0.2.1", "0")
        assertEquals(AddressProblem.OutsideTailnet("192.0.2.1"), parsed.addressProblem)
        assertEquals(PortProblem, parsed.portProblem)
    }

    @Test
    fun theAddressIsWrittenWithItsPortOnlyWhenItIsNotTheAgents() {
        assertEquals("laptop", MachineAddress("laptop", AGENT_PORT).toString())
        assertEquals("100.64.0.2", MachineAddress("100.64.0.2", AGENT_PORT).toString())
        assertEquals("fd7a:115c:a1e0::1", MachineAddress("fd7a:115c:a1e0::1", AGENT_PORT).toString())
        assertEquals("laptop:9000", MachineAddress("laptop", 9000).toString())
        assertEquals("[fd7a:115c:a1e0::1]:9000", MachineAddress("fd7a:115c:a1e0::1", 9000).toString())
    }

    @Test
    fun thePulseUrlIsTheAgentsPulsePath() {
        fun url(address: String, port: String = "") = parseAddress(address, port).address?.pulseUrl.toString()
        assertEquals("http://laptop:7247/v1/pulse", url("laptop"))
        assertEquals("http://laptop.example.ts.net:7247/v1/pulse", url(" laptop.example.ts.net. "))
        assertEquals("http://100.64.0.2:9000/v1/pulse", url("100.64.0.2", "9000"))
        assertEquals("http://[fd7a:115c:a1e0::1]:7247/v1/pulse", url("[fd7a:115c:a1e0::1]"))
    }
}
